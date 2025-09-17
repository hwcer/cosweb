package cosweb

import (
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hwcer/cosgo/binder"
	"github.com/hwcer/cosgo/registry"
	"github.com/hwcer/cosgo/scc"
	"github.com/hwcer/logger"
	"golang.org/x/net/context"
)

type (
	// Server is the top-level framework instance.
	Server struct {
		pool            sync.Pool
		status          int32            //是否已经完成注册
		middleware      []MiddlewareFunc //全局中间件
		Binder          binder.Binder    //默认序列化方式
		Render          Render
		Server          *http.Server
		Router          *registry.Router
		Registry        *registry.Registry
		RequestDataType RequestDataTypeMap //使用GET获取数据时默认的查询方式
		//HTTPErrorHandler HTTPErrorHandler
	}
	// HandlerFunc defines a function to serve HTTP requests.
	HandlerFunc func(*Context) error
	// HTTPErrorHandler is a centralized HTTP error handler.
	HTTPErrorHandler func(*Context, error)
)

var (
	AnyHttpMethod = []string{
		http.MethodGet,
		http.MethodHead,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodConnect,
		http.MethodOptions,
		http.MethodTrace,
	}
)

// New creates an instance of Server.
func New() (s *Server) {
	s = &Server{
		pool:     sync.Pool{},
		Binder:   binder.New(binder.MIMEJSON),
		Server:   new(http.Server),
		Router:   registry.NewRouter(),
		Registry: registry.New(nil),
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
func (srv *Server) GET(path string, h HandlerFunc) {
	srv.Register(path, h, http.MethodGet)
}

// POST registers a new POST Register for a path with matching handler in the
// Router with optional Register-level middleware.
func (srv *Server) POST(path string, h HandlerFunc) {
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
		method = append(AnyHttpMethod, http.MethodGet)
	}
	for _, r := range static.Route() {
		srv.Register(r, static.handle, method...)
	}
	return static
}

// Service 使用Registry的Service批量注册struct
func (srv *Server) Service(name string, handler ...interface{}) *registry.Service {
	service := srv.Registry.Service(name)
	if service.Handler == nil {
		service.Handler = &Handler{}
	}
	for _, i := range handler {
		service.Use(i)
	}
	return service
}

// Register AddTarget registers a new Register for an HTTP value and path with matching handler
// in the Router with optional Register-level middleware.
func (srv *Server) Register(route string, handler HandlerFunc, method ...string) {
	if len(method) == 0 {
		method = []string{http.MethodGet, http.MethodPost}
	}
	for _, m := range method {
		if err := srv.Router.Register(handler, strings.ToUpper(m), route); err != nil {
			logger.Alert(err)
		}
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
			Errorf(c, NewHTTPError500(e))
		}
		srv.Release(c)
	}()

	if scc.Stopped() {
		Errorf(c, errors.New("server stopped"))
		return
	}
	//srv
	if !c.doMiddleware(srv.middleware) {
		return
	}
	path := registry.Formatter(c.Request.URL.Path)
	nodes := srv.Router.Match(c.Request.Method, path)
	if err := c.doHandle(nodes); err != nil {
		Errorf(c, err)
	}
}

// Listen starts an HTTP server.
func (srv *Server) Listen(address string, tlsConfig ...*tls.Config) (err error) {
	if err = srv.register(); err != nil {
		return err
	}
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
	if err = srv.register(); err != nil {
		return err
	}
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
	if err = srv.register(); err != nil {
		return err
	}
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

// register 注册所有 service
func (srv *Server) register() error {
	if !atomic.CompareAndSwapInt32(&srv.status, 0, 1) {
		return errors.New("server already running")
	}
	srv.Registry.Nodes(func(node *registry.Node) bool {
		if handler, ok := node.Service.Handler.(*Handler); ok {
			path := registry.Join(node.Service.Name(), node.Name())
			handle := handler.closure(node)
			srv.Register(path, handle, handler.method...)
		}
		return true
	})
	return nil
}
