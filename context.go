package cosweb

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/hwcer/cosgo/binder"
	"github.com/hwcer/cosgo/registry"
	"github.com/hwcer/cosgo/session"
	"github.com/hwcer/cosgo/values"
)

const (
	indexPage     = "index.html"
	defaultMemory = 10 << 20 // 10 MB
)

// Context API上下文.
type Context struct {
	body     []byte
	query    url.Values
	route    []string
	params   map[string]string
	values   values.Values
	context  map[string]any //临时设置数据
	Server   *Server
	Binder   binder.Binder
	Session  *session.Session
	Request  *http.Request
	Response http.ResponseWriter
}

// NewContext returns a Context instance.
func NewContext(s *Server) *Context {
	c := &Context{
		Server: s,
	}
	c.Session = session.New()
	return c
}

func (c *Context) reset(w http.ResponseWriter, r *http.Request) {
	c.Binder = c.Server.Binder
	c.Request = r
	c.Response = w
}

// 释放资源,准备进入缓存池
func (c *Context) release() {
	c.body = nil
	c.query = nil
	c.route = nil
	c.params = nil
	c.values = nil
	c.context = nil
	c.Request = nil
	c.Response = nil
	c.Session.Release()
}

func (c *Context) doHandle(nodes []*registry.Router) (err error) {
	if len(nodes) == 0 {
		return ErrNotFound
	}
	node := nodes[0]
	handle, ok := node.Handle().(HandlerFunc)
	if !ok {
		return ErrHandlerError
	}
	c.params = node.Params(c.Request.URL.Path)
	return handle(c)

}

func (c *Context) doMiddleware(middleware []MiddlewareFunc) bool {
	if len(middleware) == 0 {
		return true
	}
	next := false
	var fn = func() error { next = true; return nil }
	for _, mf := range middleware {
		if err := mf(c, fn); err != nil {
			Errorf(c, err)
			return false
		} else if !next {
			return false
		} else {
			next = false
		}
	}
	return true
}

// Route 当期匹配的路由
func (c *Context) Route() string {
	return strings.Join(c.route, "/")
}

// IsWebSocket 判断是否WebSocket
func (c *Context) IsWebSocket() bool {
	upgrade := c.Request.Header.Get(HeaderUpgrade)
	return strings.ToLower(upgrade) == "websocket"
}

// Protocol 协议
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

// RemoteAddr 客户端地址
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

func (c *Context) Set(key string, val any) {
	if c.context == nil {
		c.context = make(map[string]any)
	}
	c.context[key] = val
}

// Get 获取参数,优先路径中的params
// 其他方式直接使用c.Request...
func (c *Context) Get(key string, dts ...RequestDataType) interface{} {
	if len(dts) == 0 {
		dts = c.Server.RequestDataType
	}
	for _, t := range dts {
		if v, ok := getDataFromRequest(c, key, t); ok {
			return v
		}
	}
	return ""
}
func (c *Context) GetInt(key string, dts ...RequestDataType) (r int) {
	return int(c.GetInt64(key, dts...))
}
func (c *Context) GetInt64(key string, dts ...RequestDataType) (r int64) {
	v := c.Get(key, dts...)
	if v == nil {
		return 0
	}
	return values.ParseInt64(v)
}
func (c *Context) GetInt32(key string, dts ...RequestDataType) (r int32) {
	return int32(c.GetInt64(key, dts...))
}
func (c *Context) GetFloat(key string, dts ...RequestDataType) (r float64) {
	v := c.Get(key, dts...)
	if v == nil {
		return 0
	}
	return values.ParseFloat64(v)
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

func (c *Context) Values() values.Values {
	if c.values == nil {
		c.values = values.Values{}
		_ = c.Bind(&c.values)
	}
	return c.values
}

// Bind 绑定JSON XML
func (c *Context) Bind(i any) (err error) {
	t := c.Request.Header.Get(HeaderContentType)
	encoder := binder.Get(t)
	if encoder == nil {
		return values.Errorf(0, "unknown content type: %s", t)
	}
	var b *bytes.Buffer
	if b, err = c.Buffer(); err != nil {
		return err
	}
	if b.Len() > 0 {
		err = encoder.Unmarshal(b.Bytes(), i)
	}
	return err
}

// Buffer 获取绑定body bytes
func (c *Context) Buffer() (b *bytes.Buffer, err error) {
	if c.body != nil {
		return bytes.NewBuffer(c.body), nil
	}
	b = bytes.NewBuffer([]byte{})
	defer func() {
		c.body = b.Bytes()
	}()
	var n int64
	n, err = b.ReadFrom(c.Request.Body)
	if err != nil {
		return
	}
	if n == 0 {
		if t := c.Request.Header.Get(HeaderContentType); strings.HasPrefix(strings.ToLower(t), binder.MIMEPOSTForm) {
			b.WriteString(c.Request.URL.RawQuery)
		}
	}
	return
}
