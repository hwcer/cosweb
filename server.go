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
	"sync/atomic"
	"time"
)

type (
	// Server is the top-level framework instance.
	Server struct {
		pool             sync.Pool
		status           int32            //是否已经完成注册
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

// Push creates an instance of Server.
func NewServer() (e *Server) {
	e = &Server{
		pool:     sync.Pool{},
		Binder:   binder.New(binder.MIMEJSON),
		Server:   new(http.Server),
		Router:   registry.NewRouter(),
		Registry: registry.New(nil),
	}
	e.Server.Handler = e
	//e.SessionDataType = defaultSessionDataType
	e.RequestDataType = defaultRequestDataType
	e.HTTPErrorHandler = e.DefaultHTTPErrorHandler
	//e.Binder = &DefaultBinder{}
	e.pool.New = func() interface{} {
		return NewContext(e)
	}
	//e.Router = NewRouter()
	return
}

// DefaultHTTPErrorHandler is the default HTTP error handler. It sends a JSON Response
// with status code.
func (s *Server) DefaultHTTPErrorHandler(c *Context, err error) {
	he, ok := err.(*HTTPError)
	if !ok {
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
func (s *Server) Use(middleware ...MiddlewareFunc) {
	s.middleware = append(s.middleware, middleware...)
}

// GET registers a new GET Register for a path with matching handler in the Router
// with optional Register-level middleware.
func (s *Server) GET(path string, h HandlerFunc) {
	s.Register(path, h, http.MethodGet)
}

// POST registers a new POST Register for a path with matching handler in the
// Router with optional Register-level middleware.
func (s *Server) POST(path string, h HandlerFunc) {
	s.Register(path, h, http.MethodPost)
}

// 代理服务器
func (s *Server) Proxy(prefix, address string, method ...string) *Proxy {
	proxy := NewProxy(address)
	proxy.Route(s, prefix, method...)
	return proxy
}

// Static registers a new Register with path prefix to serve static files from the
// provided root directory.
// 如果root 不是绝对路径 将以程序的WorkDir为根目录
func (s *Server) Static(prefix, root string, method ...string) *Static {
	static := NewStatic(root)
	static.Route(s, prefix, method...)
	return static
}

// Service 使用Registry的Service批量注册struct
func (this *Server) Service(name string, handler ...interface{}) *registry.Service {
	service := this.Registry.Service(name)
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
func (s *Server) Register(route string, handler HandlerFunc, method ...string) {
	if len(method) == 0 {
		method = []string{http.MethodGet, http.MethodPost}
	}
	for _, m := range method {
		_ = s.Router.Register(handler, strings.ToUpper(m), route)
	}
}

// Acquire returns an empty `Context` instance from the pool.
// You must return the Context by calling `ReleaseContext()`.
func (s *Server) Acquire(w http.ResponseWriter, r *http.Request) *Context {
	c := s.pool.Get().(*Context)
	c.reset(w, r)
	return c
}

// Release returns the `Context` instance back to the pool.
// You must call it after `AcquireContext()`.
func (s *Server) Release(c *Context) {
	c.release()
	s.pool.Put(c)
}

// ServeHTTP implements `http.Handler` interface, which serves HTTP requests.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c := s.Acquire(w, r)
	defer func() {
		if e := recover(); e != nil {
			s.HTTPErrorHandler(c, NewHTTPError500(e))
		}
		s.Release(c)
	}()

	if scc.Stopped() {
		s.HTTPErrorHandler(c, errors.New("server stopped"))
		return
	}

	if err, ok := c.doMiddleware(s.middleware); err != nil {
		s.HTTPErrorHandler(c, err)
		return
	} else if !ok {
		return
	}

	nodes := s.Router.Match(c.Request.Method, c.Request.URL.Path)
	err := c.doHandle(nodes)
	if err != nil {
		s.HTTPErrorHandler(c, err)
	} else if c.aborted == 0 {
		s.HTTPErrorHandler(c, ErrNotFound) //所有备选路由都放弃执行
	}
}

// Start starts an HTTP server.
func (s *Server) Start(address string, tlsConfig ...*tls.Config) (err error) {
	s.register()
	s.Server.Addr = address
	if len(tlsConfig) > 0 {
		s.Server.TLSConfig = tlsConfig[0]
	}
	//启动服务
	err = scc.Timeout(time.Second, func() error {
		if s.Server.TLSConfig != nil {
			return s.Server.ListenAndServeTLS("", "")
		} else {
			return s.Server.ListenAndServe()
		}
	})
	if err == scc.ErrorTimeout {
		err = nil
	}
	return
}

func (s *Server) Close() {
	if !atomic.CompareAndSwapInt32(&s.status, 1, 0) {
		return
	}
	_ = s.Server.Shutdown(context.Background())
	_ = session.Close()
	scc.Done()
}

func (s *Server) Listener(ln net.Listener) (err error) {
	s.register()
	//s.Server = &http.Server{Handler: cors.Default().Handler(s)}
	//s.Server.Handler =cors.Default().Handler(s)
	//启动服务
	err = scc.Timeout(time.Second, func() error {
		return s.Server.Serve(ln)
	})
	if err == scc.ErrorTimeout {
		err = nil
	}
	return
}

// register 注册所有 service
func (s *Server) register() {
	if !atomic.CompareAndSwapInt32(&s.status, 0, 1) {
		return
	}
	scc.Add(1)
	scc.CGO(s.heartbeat)
	//scc.Add(1)
	s.Registry.Nodes(func(node *registry.Node) bool {
		if handler, ok := node.Service.Handler.(*Handler); ok {
			path := registry.Join(node.Service.Name(), node.Name())
			s.Register(path, handler.closure(node), handler.method...)
		}
		return true
	})
}

func (this *Server) heartbeat(ctx context.Context) {
	defer this.Close()
	for {
		select {
		case <-ctx.Done():
			return
		}
	}
}
