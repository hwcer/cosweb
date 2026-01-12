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
	accept   binder.Binder //客户端接受的序列化方式
	params   map[string]string
	values   values.Values
	context  map[string]any //临时设置数据
	Server   *Server
	Session  *session.Session
	Request  *http.Request
	Response *Response
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
	c.Request = r
	c.Response = &Response{ResponseWriter: w, canWrite: true}
}

// 释放资源,准备进入缓存池
func (c *Context) release() {
	c.body = nil
	c.query = nil
	c.accept = nil
	c.params = nil
	c.values = nil
	c.context = nil
	c.Request = nil
	c.Response = nil
	c.Session.Release()
}

func (c *Context) doHandle(nodes []*registry.Node) error {
	if len(nodes) == 0 {
		return ErrNotFound
	}
	node := nodes[0]
	handle, ok := node.Handler().(*Handler)
	if !ok {
		return ErrHandlerError
	}
	c.params = node.Params(c.Request.URL.Path)

	if err := c.doMiddleware(handle.middleware); err != nil {
		return err
	}
	if reply, err := handle.handle(node, c); err != nil {
		return err
	} else {
		return handle.write(c, reply)
	}
}

func (c *Context) doMiddleware(middleware []MiddlewareFunc) (err error) {
	if len(middleware) == 0 {
		return nil
	}
	next := false
	var cb Next = func() error {
		next = true
		return nil
	}
	for _, mf := range middleware {
		if err = mf(c, cb); err != nil {
			return
		}
		if !next {
			return nil
		}
		next = false
	}
	return
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

func (c *Context) Error(format any) error {
	return values.Error(format)
}

// Errorf 封装一个错误
func (c *Context) Errorf(code int32, format any, args ...any) error {
	return values.Errorf(code, format, args...)
}

func (this *Context) Accept() binder.Binder {
	if this.accept != nil {
		return this.accept
	}
	var arr []string
	if header := this.Request.Header.Get(HeaderAccept); header != "" {
		arr = strings.Split(header, ",")
	}

	if header := this.Request.Header.Get(HeaderContentType); header == "" {
		arr = append(arr, strings.Split(header, ",")...)
	}

	for _, s := range arr {
		if this.accept = binder.Get(s); this.accept != nil {
			return this.accept
		}
	}
	this.accept = this.Server.Binder
	return this.accept
}
