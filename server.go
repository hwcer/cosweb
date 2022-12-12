package cosweb

import (
	ctx "context"
	"crypto/tls"
	"errors"
	"github.com/hwcer/cosgo/binder"
	"github.com/hwcer/cosgo/logger"
	"github.com/hwcer/cosgo/registry"
	"github.com/hwcer/cosgo/utils"
	"github.com/hwcer/cosweb/session"
	"net/http"
	"strings"
	"sync"
	"time"
)

type (
	// Server is the top-level framework instance.
	Server struct {
		scc              *utils.SCC
		pool             sync.Pool
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
func NewServer(tlsConfig ...*tls.Config) (e *Server) {
	e = &Server{
		scc:      utils.NewSCC(nil),
		pool:     sync.Pool{},
		Binder:   binder.New(binder.MIMEJSON),
		Server:   new(http.Server),
		Router:   registry.NewRouter(),
		Registry: registry.New(nil),
	}
	if len(tlsConfig) > 0 {
		e.Server.TLSConfig = tlsConfig[0]
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
	if he.Code != http.StatusNotFound {
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

// AddTarget registers a new Register for an HTTP value and path with matching handler
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
	s.scc.Add(1)
	c := s.Acquire(w, r)
	defer func() {
		if e := recover(); e != nil {
			s.HTTPErrorHandler(c, NewHTTPError500(e))
		}
		s.scc.Done()
		s.Release(c)
	}()

	if s.scc.Stopped() {
		s.HTTPErrorHandler(c, errors.New("server stopped"))
		return
	}
	if c.Body.errors != nil {
		s.HTTPErrorHandler(c, NewHTTPError500(c.Body.errors))
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
func (s *Server) Start(address string) (err error) {
	s.scc.Add(1)
	s.Server.Addr = address
	//注册所有 service
	s.Registry.Range(func(service *registry.Service, node *registry.Node) bool {
		if handler, ok := service.Handler.(*Handler); ok {
			path := registry.Join(service.Name(), node.Name())
			s.Register(path, handler.closure(node), handler.method...)
		}
		return true
	})
	//启动服务
	err = utils.Timeout(time.Second, func() error {
		if s.Server.TLSConfig != nil {
			return s.Server.ListenAndServeTLS("", "")
		} else {
			return s.Server.ListenAndServe()
		}
	})
	if err != nil && err != utils.ErrorTimeout {
		return err
	}
	return nil
}

// Close 立即关闭
func (s *Server) Close() error {
	s.scc.Done()
	if s.scc.Cancel() {
		_ = s.scc.Wait(10 * time.Second)
	}
	err := s.Server.Close()
	if err == nil {
		err = session.Close()
	}
	return err
}

// Shutdown 优雅关闭，等所有协程结束
func (s *Server) Shutdown(ctx ctx.Context) error {
	err := s.Server.Shutdown(ctx)
	if err == nil {
		err = session.Close()
	}
	return err
}
