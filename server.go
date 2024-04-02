package cosweb

import (
	"crypto/tls"
	"errors"
	"github.com/hwcer/cosgo/binder"
	"github.com/hwcer/cosweb/session"
	"github.com/hwcer/logger"
	"github.com/hwcer/registry"
	"github.com/hwcer/scc"
	"golang.org/x/net/context"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type (
	// Server is the top-level framework instance.
	Server struct {
		SCC  *scc.SCC
		pool sync.Pool
		///status           int32            //是否已经完成注册
		middleware       []MiddlewareFunc //中间件
		Binder           binder.Interface //默认序列化方式
		Render           Render
		Server           *http.Server
		Router           *registry.Router
		Registry         *registry.Registry
		RequestDataType  RequestDataTypeMap //使用GET获取数据时默认的查询方式
		HTTPErrorHandler HTTPErrorHandler
	}
	Next func() error
	// HandlerFunc defines a function to serve HTTP requests.
	HandlerFunc func(*Context, Next) error
	// MiddlewareFunc defines a function to process middleware.
	MiddlewareFunc func(*Context, Next) error
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
func New(ctx context.Context) (s *Server) {
	s = &Server{
		SCC:      scc.New(ctx),
		pool:     sync.Pool{},
		Binder:   binder.New(binder.MIMEJSON),
		Server:   new(http.Server),
		Router:   registry.NewRouter(),
		Registry: registry.New(nil),
	}
	s.Server.Handler = s
	s.RequestDataType = defaultRequestDataType
	s.HTTPErrorHandler = s.DefaultHTTPErrorHandler
	s.pool.New = func() interface{} {
		return NewContext(s)
	}
	return
}

// DefaultHTTPErrorHandler is the default HTTP error handler. It sends a JSON Response
// with status code.
func (srv *Server) DefaultHTTPErrorHandler(c *Context, err error) {
	he := &HTTPError{}
	if !errors.As(err, &he) {
		he = NewHTTPError(http.StatusInternalServerError, err)
	}
	c.WriteHeader(he.Code)
	if c.Request.Method != http.MethodHead {
		_ = c.Bytes(ContentTypeTextPlain, []byte(he.String()))
	}
	if he.Code != http.StatusNotFound && he.Code != http.StatusInternalServerError {
		logger.Error(he)
	}
}

// Use adds middleware to the chain which is run after Router.
func (srv *Server) Use(middleware ...MiddlewareFunc) {
	srv.middleware = append(srv.middleware, middleware...)
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
	static := NewStatic(root)
	static.Route(srv, prefix, method...)
	return static
}

// Service 使用Registry的Service批量注册struct
func (srv *Server) Service(name string, handler ...interface{}) *registry.Service {
	service := srv.Registry.Service(name)
	if service.Handler == nil {
		service.Handler = &Handler{}
	}
	if h, ok := service.Handler.(*Handler); ok {
		for _, i := range handler {
			h.Use(i)
		}
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
		_ = srv.Router.Register(handler, strings.ToUpper(m), route)
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
	srv.SCC.Add(1)
	defer srv.SCC.Done()
	c := srv.Acquire(w, r)
	defer func() {
		if e := recover(); e != nil {
			srv.HTTPErrorHandler(c, NewHTTPError500(e))
		}
		srv.Release(c)
	}()

	if srv.SCC.Stopped() {
		srv.HTTPErrorHandler(c, errors.New("server stopped"))
		return
	}
	if err, ok := c.doMiddleware(srv.middleware); err != nil {
		srv.HTTPErrorHandler(c, err)
		return
	} else if !ok {
		return
	}

	nodes := srv.Router.Match(c.Request.Method, c.Request.URL.Path)
	err := c.doHandle(nodes)
	if err != nil {
		srv.HTTPErrorHandler(c, err)
	} else if c.aborted == 0 {
		srv.HTTPErrorHandler(c, ErrNotFound) //所有备选路由都放弃执行
	}
}

// Start starts an HTTP server.
func (srv *Server) Start(address string, tlsConfig ...*tls.Config) (err error) {
	srv.register()
	srv.Server.Addr = address
	if len(tlsConfig) > 0 {
		srv.Server.TLSConfig = tlsConfig[0]
	}
	//启动服务
	err = srv.SCC.Timeout(time.Second, func() error {
		if srv.Server.TLSConfig != nil {
			return srv.Server.ListenAndServeTLS("", "")
		} else {
			return srv.Server.ListenAndServe()
		}
	})
	if errors.Is(err, scc.ErrorTimeout) {
		err = nil
	}
	return
}

func (srv *Server) Listen(ln net.Listener) (err error) {
	srv.register()
	//启动服务
	err = srv.SCC.Timeout(time.Second, func() error {
		return srv.Server.Serve(ln)
	})
	if errors.Is(err, scc.ErrorTimeout) {
		err = nil
	}
	return
}

func (srv *Server) Close() error {
	if !srv.SCC.Cancel() {
		return nil
	}
	if err := srv.SCC.Wait(time.Second); err != nil {
		return err
	}
	_ = srv.Server.Shutdown(context.Background())
	_ = session.Close()
	return nil
}

// register 注册所有 service
func (srv *Server) register() {
	if !srv.SCC.Stopped() {
		return
	}
	srv.Registry.Nodes(func(node *registry.Node) bool {
		if handler, ok := node.Service.Handler.(*Handler); ok {
			path := registry.Join(node.Service.Name(), node.Name())
			srv.Register(path, handler.closure(node), handler.method...)
		}
		return true
	})
}
