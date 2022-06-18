package cosweb

import (
	"fmt"
	"github.com/hwcer/cosweb/session"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	indexPage     = "index.html"
	defaultMemory = 10 << 20 // 10 MB
)

type CCPool struct {
	data    interface{}
	reset   func()
	release func()
}

//Context API上下文.
type Context struct {
	pool     *CCPool //缓存逻辑层对象
	query    url.Values
	params   map[string]string
	engine   *Server
	aborted  int
	Body     *Body
	Cookie   *Cookie
	Session  *session.Session
	Request  *http.Request
	Response http.ResponseWriter
}

// NewContext returns a Context instance.
func NewContext(s *Server) *Context {
	c := &Context{
		engine: s,
	}
	c.Body = NewBody(c)
	c.Cookie = NewCookie(c)
	c.Session = session.New()
	return c
}

func (c *Context) reset(w http.ResponseWriter, r *http.Request) {
	c.Request = r
	c.Response = w
	c.Session.Reset(c.GetString(session.Options.Name, c.engine.SessionDataType...))
}

//释放资源,准备进入缓存池
func (c *Context) release() {
	c.query = nil
	c.params = nil
	c.aborted = 0
	c.Request = nil
	c.Response = nil
	c.Body.release()
	c.Cookie.release()
	c.Session.Release()
}

func (c *Context) next() error {
	c.aborted -= 1
	return nil
}

func (c *Context) doHandle(nodes []*Node) (err error) {
	if len(nodes) == 0 {
		return
	}
	c.aborted += len(nodes)
	num := c.aborted
	for _, node := range nodes {
		num -= 1
		c.params = node.Params(c.Request.URL.Path)
		err = node.Handler(c, c.next)
		if err != nil || c.aborted != num {
			return
		}
	}
	return
}

//doMiddleware 执行中间件
func (c *Context) doMiddleware(middleware []MiddlewareFunc) (error, bool) {
	if len(middleware) == 0 {
		return nil, true
	}
	c.aborted += len(middleware)
	num := c.aborted
	for _, modFun := range middleware {
		num -= 1
		if err := modFun(c, c.next); err != nil {
			return err, false
		}
		if c.aborted != num {
			return nil, false
		}
	}
	return nil, true
}

func (c *Context) Abort() {
	c.aborted += 1
}

//Pool 获取缓存池中缓存的对象
func (c *Context) Pool() (i interface{}) {
	if c.pool != nil {
		i = c.pool.data
	}
	return
}

//IsWebSocket 判断是否WebSocket
func (c *Context) IsWebSocket() bool {
	upgrade := c.Request.Header.Get(HeaderUpgrade)
	return strings.ToLower(upgrade) == "websocket"
}

//Protocol 协议
func (c *Context) Protocol() string {
	// Can't use `r.Request.URL.Protocol`
	// See: https://groups.google.com/forum/#!topic/golang-nuts/pMUkBlQBDF0
	if c.Request.TLS != nil {
		return "https"
	}
	if scheme := c.Request.Header.Get(HeaderXForwardedProto); scheme != "" {
		return scheme
	}
	if scheme := c.Request.Header.Get(HeaderXForwardedProtocol); scheme != "" {
		return scheme
	}
	if ssl := c.Request.Header.Get(HeaderXForwardedSsl); ssl == "on" {
		return "https"
	}
	if scheme := c.Request.Header.Get(HeaderXUrlScheme); scheme != "" {
		return scheme
	}
	return "http"
}

//RemoteAddr 客户端地址
func (c *Context) RemoteAddr() string {
	// Fall back to legacy behavior
	if ip := c.Request.Header.Get(HeaderXForwardedFor); ip != "" {
		return strings.Split(ip, ", ")[0]
	}
	if ip := c.Request.Header.Get(HeaderXRealIP); ip != "" {
		return ip
	}
	ra, _, _ := net.SplitHostPort(c.Request.RemoteAddr)
	return ra
}

//Get 获取参数,优先路径中的params
//其他方式直接使用c.Request...
func (c *Context) Get(key string, dts ...RequestDataType) interface{} {
	if len(dts) == 0 {
		dts = c.engine.RequestDataType
	}
	for _, t := range dts {
		if v, ok := getDataFromRequest(c, key, t); ok {
			return v
		}
	}
	return ""
}
func (c *Context) GetInt(key string, dts ...RequestDataType) (r int64) {
	v := c.Get(key, dts...)
	if v == nil {
		return 0
	}
	switch v.(type) {
	case int:
		r = int64(v.(int))
	case int32:
		r = int64(v.(int32))
	case int64:
		r = int64(v.(int64))
	case float32:
		r = int64(v.(float32))
	case float64:
		r = int64(v.(float64))
	case string:
		r, _ = strconv.ParseInt(v.(string), 10, 64)
	}
	return
}

func (c *Context) GetFloat(key string, dts ...RequestDataType) (r float64) {
	v := c.Get(key, dts...)
	if v == nil {
		return 0
	}
	switch v.(type) {
	case int:
		r = float64(v.(int))
	case int32:
		r = float64(v.(int32))
	case int64:
		r = float64(v.(int64))
	case float32:
		r = float64(v.(float32))
	case float64:
		r = v.(float64)
	case string:
		r, _ = strconv.ParseFloat(v.(string), 10)
	}
	return
}

func (c *Context) GetString(key string, dts ...RequestDataType) (r string) {
	v := c.Get(key, dts...)
	if v == nil {
		return ""
	}
	switch v.(type) {
	case string:
		r = v.(string)
	default:
		r = fmt.Sprintf("%v", v)
	}
	return
}

//Bind 绑定JSON XML
func (c *Context) Bind(i interface{}) error {
	return c.Body.Bind(i)
}
