package cosweb

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
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
	AcceptIgnore    map[string]bool    //响应协商时忽略的 MIME 类型（如 */*、form-urlencoded）
	RequestDataType RequestDataTypeMap //使用GET获取数据时默认的查询方式
	MaxBodySize     int64              //最大请求体大小，默认 10MB
	MaxCacheSize    int64              //最大缓存大小，默认 1MB
	mutex           sync.Mutex         //保护 listeners
	listeners       []net.Listener     //已启动的监听器,支持多端口
	trigger         sync.Once          //保证 shutdown 回调只注册一次
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
		AcceptIgnore: map[string]bool{"*/*": true, binder.MIMEPOSTForm: true},
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

// Proxy 注册反向代理，通配路由匹配 prefix 下所有路径
func (srv *Server) Proxy(prefix, address string, method ...string) *Proxy {
	proxy := NewProxy(address)
	srv.Register(wildcardRoute(prefix), proxy.Handle, method...)
	return proxy
}

// Static 注册静态文件服务，通配路由匹配 prefix 下所有路径
// 如果 root 不是绝对路径，以程序的 WorkDir 为根目录
func (srv *Server) Static(prefix, root string, method ...string) *Static {
	static := NewStatic(root)
	if len(method) == 0 {
		method = []string{http.MethodGet, http.MethodHead}
	}
	srv.Register(wildcardRoute(prefix), static.Handle, method...)
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

func (srv *Server) Handler(name ...string) (r *Handler) {
	var s string
	if len(name) > 0 {
		s = name[0]
	}
	service := srv.Registry.Service(s, &Handler{})
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
	c := srv.Acquire(w, r)
	defer func() {
		if e := recover(); e != nil {
			HTTPErrorHandler(c, e)
		}
		srv.Release(c)
		scc.Done()
	}()

	if scc.Stopped() {
		HTTPErrorHandler(c, "server stopped")
		return
	}
	// 1. global middleware
	funcs := append([]MiddlewareFunc{}, srv.middleware...)

	// 2. path service handler middleware (e.g. /ws WebSocket middleware)
	path := c.Request.URL.Path
	var pathHandler *Handler
	if service, _ := srv.Registry.Get(path); service != nil {
		if h, ok := service.GetHandler().(*Handler); ok {
			pathHandler = h
			funcs = append(funcs, h.middleware...)
		}
	}

	// 3. search route node
	c.node, c.params = srv.Registry.Search(c.Request.Method, path)
	var nodeHandler *Handler
	if c.node != nil {
		if h, ok := c.node.Handler().(*Handler); ok {
			nodeHandler = h
		}
	}

	// 4. if node handler differs from path handler, append node handler middleware
	if nodeHandler != nil && nodeHandler != pathHandler && len(nodeHandler.middleware) > 0 {
		funcs = append(funcs, nodeHandler.middleware...)
	}

	c.dp.funcs = funcs

	if err := c.doDispatch(); err != nil {
		HTTPErrorHandler(c, err)
	}
}

// bind 同步绑定一个端点,不启动 Serve。
// 与 ListenAndServe 不同,net.Listen 的错误(端口占用、权限不足)立即返回,
// 不需要靠超时探测。
func (srv *Server) bind(ep Endpoint) (net.Listener, error) {
	network := ep.Network
	if network == "" {
		network = "tcp"
	}
	ml := Listener(network)
	if ml == nil {
		return nil, fmt.Errorf("unknown network: %s", network)
	}
	cfg := ep.TLS
	if cfg == nil {
		cfg = srv.Server.TLSConfig //兼容旧用法:直接设置 srv.Server.TLSConfig
	}
	return ml(ep.Address, withALPN(cfg))
}

// serve 登记 listener 并在 scc 跟踪的协程内 Serve。
// 注意不要给 srv.Server.TLSConfig 赋值:TLSConfig 为 nil 时 Serve 会自动配置 HTTP/2
// (见 net/http shouldConfigureHTTP2ForServe),赋值反而会让 TLS 端口静默丢失 h2。
func (srv *Server) serve(ln net.Listener) {
	srv.mutex.Lock()
	srv.listeners = append(srv.listeners, ln)
	if srv.Server.Addr == "" {
		srv.Server.Addr = ln.Addr().String() //信息性,仅记录首个端口
	}
	srv.mutex.Unlock()
	srv.trigger.Do(func() { scc.Trigger(srv.shutdown) })
	scc.GO(func() {
		if err := srv.Server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Alert("cosweb serve %v: %v", ln.Addr(), err)
		}
	})
}

// Listen 绑定并启动一个端口,可重复调用以同时监听多个端口,所有端口共享同一套路由。
func (srv *Server) Listen(address string, tlsConfig ...*tls.Config) error {
	ep := Endpoint{Address: address}
	if len(tlsConfig) > 0 {
		ep.TLS = tlsConfig[0]
	}
	return srv.ListenAll(ep)
}

// ListenAll 原子批量绑定:先全部 bind,任一失败则关闭本批已绑定的 listener 并返回错误,
// 全部成功后才开始 Serve。
// 回滚边界是单次调用,之前 Listen 已启动的端口不受影响;需要跨端口原子性就一次传完。
func (srv *Server) ListenAll(endpoint ...Endpoint) error {
	lns := make([]net.Listener, 0, len(endpoint))
	for _, ep := range endpoint {
		ln, err := srv.bind(ep)
		if err != nil {
			for _, l := range lns {
				_ = l.Close()
			}
			return err
		}
		lns = append(lns, ln)
	}
	for _, ln := range lns {
		srv.serve(ln)
	}
	return nil
}

// TLS starts an HTTPS server.
// address  string | net.Listener
func (srv *Server) TLS(address any, certFile, keyFile string) (err error) {
	var cfg *tls.Config
	if cfg, err = TLSConfigParse(certFile, keyFile); err != nil {
		return
	}
	switch v := address.(type) {
	case string:
		return srv.ListenAll(Endpoint{Address: v, TLS: cfg})
	case net.Listener:
		srv.serve(tls.NewListener(v, withALPN(cfg)))
		return nil
	default:
		return errors.New("unknown address type")
	}
}

// Accept 在一个已有的 listener 上提供服务,可重复调用。
func (srv *Server) Accept(ln net.Listener) error {
	srv.serve(ln)
	return nil
}

// Addresses 返回全部实际监听地址,使用 :0 随机端口时可用它取真实端口。
func (srv *Server) Addresses() []net.Addr {
	srv.mutex.Lock()
	defer srv.mutex.Unlock()
	r := make([]net.Addr, 0, len(srv.listeners))
	for _, ln := range srv.listeners {
		r = append(r, ln.Addr())
	}
	return r
}

func wildcardRoute(prefix string) string {
	if strings.HasSuffix(prefix, "*") {
		return prefix
	}
	return strings.TrimRight(prefix, "/") + "/*"
}

func (srv *Server) shutdown() {
	_ = srv.Server.Shutdown(context.Background())
}
