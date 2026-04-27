package cosweb

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hwcer/cosgo/binder"
	"github.com/hwcer/cosgo/registry"
	"github.com/hwcer/cosgo/scc"
	"github.com/hwcer/logger"
)

// Server is the top-level framework instance.
type Server struct {
	pool            sync.Pool
	middleware      []MiddlewareFunc //全局中间件
	Binder          binder.Binder    //默认序列化方式
	Render          Render
	Server          *http.Server
	Registry        *registry.Registry
	RequestDataType RequestDataTypeMap //使用GET获取数据时默认的查询方式
	MaxBodySize     int64              //最大请求体大小，默认 10MB
	MaxCacheSize    int64              //最大缓存大小，默认 1MB
}

var (
	AnyHttpMethod = []string{
		http.MethodGet,
		http.MethodHead,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodOptions,
	}
)

// 默认超时,防 Slowloris 等慢速攻击。用户可通过 s.Server.Xxx 覆盖。
const (
	defaultReadHeaderTimeout = 20 * time.Second
	defaultIdleTimeout       = 60 * time.Second
)

// New creates an instance of Server.
func New() (s *Server) {
	s = &Server{
		Binder: binder.New(binder.MIMEJSON),
		Server: &http.Server{
			ReadHeaderTimeout: defaultReadHeaderTimeout,
			IdleTimeout:       defaultIdleTimeout,
		},
		Registry:     registry.New(),
		MaxBodySize:  10 << 20, // 10 MB
		MaxCacheSize: 1 << 20,  // 1 MB
	}
	s.Server.Handler = s
	s.RequestDataType = defaultRequestDataType
	s.pool.New = func() any {
		return NewContext(s)
	}
	return
}

func (srv *Server) Use(i MiddlewareFunc) {
	if i == nil {
		return
	}
	srv.middleware = append(srv.middleware, i)
}

// GET registers a new GET Register for a path with matching handler in the Router
// with optional Register-level middleware.
func (srv *Server) GET(path string, h func(*Context) any) {
	srv.Register(path, h, http.MethodGet)
}

// POST registers a new POST Register for a path with matching handler in the
// Router with optional Register-level middleware.
func (srv *Server) POST(path string, h func(*Context) any) {
	srv.Register(path, h, http.MethodPost)
}

// Proxy 注册反向代理（全局中间件方式）
// 匹配前缀的请求转发到上游，不匹配自动回退到 API 路由
func (srv *Server) Proxy(prefix, address string, method ...string) *Proxy {
	proxy := NewProxy(address)
	if prefix != "/" {
		prefix = strings.TrimRight(prefix, "/")
	}
	proxy.prefix = prefix
	if len(method) > 0 {
		proxy.methods = make(map[string]bool, len(method))
		for _, m := range method {
			proxy.methods[m] = true
		}
	}
	srv.Use(proxy.Middleware)
	return proxy
}

// Static 注册静态文件服务（全局中间件方式）
// 文件存在直接响应，不存在自动回退到 API 路由匹配
// 如果 root 不是绝对路径，以程序的 WorkDir 为根目录
func (srv *Server) Static(prefix, root string, method ...string) *Static {
	static := NewStatic(prefix, root)
	if len(method) > 0 {
		static.methods = make(map[string]bool, len(method))
		for _, m := range method {
			static.methods[m] = true
		}
	}
	srv.Use(static.Middleware)
	return static
}

// Service 使用Registry的Service批量注册struct
func (srv *Server) Service(name ...string) *registry.Service {
	handler := &Handler{}
	var s string
	if len(name) > 0 {
		s = name[0]
	}
	service := srv.Registry.Service(s, handler)
	service.SetMethods(AnyHttpMethod)
	return service
}

func (srv *Server) Handler(name ...string) *Handler {
	var s string
	if len(name) > 0 {
		s = name[0]
	}
	service := srv.Registry.Service(s)
	return service.GetHandler().(*Handler)
}

// Register AddTarget registers a new Register for an HTTP value and path with matching handler
// in the Router with optional Register-level middleware.
func (srv *Server) Register(route string, handler func(*Context) any, method ...string) {
	service := srv.Service()
	var err error
	if len(method) == 0 {
		err = service.Register(handler, route)
	} else {
		err = service.RegisterWithMethod(handler, method, route)
	}
	if err != nil {
		logger.Alert(err)
	}
}

// Acquire returns an empty `Context` instance from the pool.
// You must return the Context by calling `ReleaseContext()`.
func (srv *Server) Acquire(w http.ResponseWriter, r *http.Request) *Context {
	c := srv.pool.Get().(*Context)
	c.reset(w, r)
	return c
}

// Release returns the `Context` instance back to the pool.
// You must call it after `AcquireContext()`.
func (srv *Server) Release(c *Context) {
	c.release()
	srv.pool.Put(c)
}

// ServeHTTP implements `http.Handler` interface, which serves HTTP requests.
func (srv *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	scc.Add(1)
	defer scc.Done()
	c := srv.Acquire(w, r)
	defer func() {
		if e := recover(); e != nil {
			HTTPErrorHandler(c, e)
		}
		srv.Release(c)
	}()

	if scc.Stopped() {
		HTTPErrorHandler(c, "server stopped")
		return
	}
	// node/params 存入 Context，避免闭包捕获产生堆分配
	c.node, c.params = srv.Registry.Search(c.Request.Method, c.Request.URL.Path)
	// 全局中间件 + handler 作为一条链执行（handler 在链尾自动触发，无额外 slice/closure 分配）
	if err := c.doMiddlewareWithHandler(srv.middleware); err != nil {
		HTTPErrorHandler(c, err)
	}
}

// Listen starts an HTTP server.
func (srv *Server) Listen(address string, tlsConfig ...*tls.Config) (err error) {
	srv.Server.Addr = address
	if len(tlsConfig) > 0 {
		srv.Server.TLSConfig = tlsConfig[0]
	}
	//启动服务
	err = scc.Timeout(time.Second, func() error {
		if srv.Server.TLSConfig != nil {
			return srv.Server.ListenAndServeTLS("", "")
		} else {
			return srv.Server.ListenAndServe()
		}
	})
	if errors.Is(err, scc.ErrorTimeout) {
		err = nil
	}
	if err == nil {
		scc.Trigger(srv.shutdown)
	}
	return
}

// TLS starts an HTTPS server.
// address  string | net.Listener
func (srv *Server) TLS(address any, certFile, keyFile string) (err error) {
	//启动服务
	err = scc.Timeout(time.Second, func() error {
		switch v := address.(type) {
		case string:
			srv.Server.Addr = v
			return srv.Server.ListenAndServeTLS(certFile, keyFile)
		case net.Listener:
			return srv.Server.ServeTLS(v, certFile, keyFile)
		default:
			return errors.New("unknown address type")
		}
	})
	if errors.Is(err, scc.ErrorTimeout) {
		err = nil
	}
	if err == nil {
		scc.Trigger(srv.shutdown)
	}
	return
}

func (srv *Server) Accept(ln net.Listener) (err error) {
	//启动服务
	err = scc.Timeout(time.Second, func() error {
		return srv.Server.Serve(ln)
	})
	if errors.Is(err, scc.ErrorTimeout) {
		err = nil
	}
	if err == nil {
		scc.Trigger(srv.shutdown)
	}
	return
}

func (srv *Server) shutdown() {
	_ = srv.Server.Shutdown(context.Background())
}
