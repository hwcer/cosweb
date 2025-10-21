package cosweb

import (
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/hwcer/cosgo/binder"
	"github.com/hwcer/cosgo/registry"
	"github.com/hwcer/cosgo/scc"
	"github.com/hwcer/logger"
	"golang.org/x/net/context"
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

// New creates an instance of Server.
func New() (s *Server) {
	s = &Server{
		pool:   sync.Pool{},
		Binder: binder.New(binder.MIMEJSON),
		Server: new(http.Server),
		//Router:   registry.NewRouter(),
		Registry: registry.New(),
	}
	s.Server.Handler = s
	s.RequestDataType = defaultRequestDataType
	s.pool.New = func() interface{} {
		return NewContext(s)
	}
	return
}

func (srv *Server) Use(i MiddlewareFunc) {
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

// 代理服务器
func (srv *Server) Proxy(prefix, address string, method ...string) *Proxy {
	proxy := NewProxy(address)
	proxy.Route(srv, prefix, method...)
	return proxy
}

// Static registers a new Register with path prefix to serve static files from the
// provided root directory.
// 如果root 不是绝对路径 将以程序的WorkDir为根目录
func (srv *Server) Static(prefix, root string, method ...string) *Static {
	static := NewStatic(prefix, root)
	if len(method) == 0 {
		method = []string{http.MethodGet}
	}
	for _, r := range static.Route() {
		srv.Register(r, static.handle, method...)
	}
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
	//srv
	if !c.doMiddleware(srv.middleware) {
		return
	}
	nodes := srv.Registry.Search(c.Request.Method, c.Request.URL.Path)
	if err := c.doHandle(nodes); err != nil {
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
