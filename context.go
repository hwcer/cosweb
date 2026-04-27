package cosweb

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/hwcer/cosgo/binder"
	"github.com/hwcer/cosgo/registry"
	"github.com/hwcer/cosgo/session"
	"github.com/hwcer/cosgo/values"
)

const (
	indexPage = "index.html"
)

// Context API上下文.
type Context struct {
	body     []byte
	accept   binder.Binder                     //客户端接受的序列化方式
	stores   map[RequestDataType]values.Values // 统一存储所有参数
	node     *registry.Node                    // 当前匹配的路由节点（避免闭包分配）
	params   registry.Params                   // 当前路径参数
	Server   *Server
	Session  *session.Session
	Request  *http.Request
	Response *Response
}

// NewContext returns a Context instance.
func NewContext(s *Server) *Context {
	c := &Context{
		Server: s,
		stores: make(map[RequestDataType]values.Values),
	}
	c.Session = session.New()
	return c
}

func (c *Context) reset(w http.ResponseWriter, r *http.Request) {
	c.Request = r
	c.Response = &Response{ResponseWriter: w}
	c.node = nil
	c.params = nil
	clear(c.stores)
}

// 释放资源,准备进入缓存池
func (c *Context) release() {
	c.body = nil
	c.accept = nil
	clear(c.stores)
	c.Request = nil
	c.Response = nil
	c.Session.Release()
}

func (c *Context) doHandle(node *registry.Node) error {
	handle, ok := node.Handler().(*Handler)
	if !ok {
		return ErrHandlerError
	}
	// 无路由级中间件时直连 handler，避免 slice + closure 分配
	if len(handle.middleware) == 0 {
		reply, err := handle.handle(node, c)
		if err != nil {
			return err
		}
		return handle.write(c, reply)
	}
	middleware := append([]MiddlewareFunc{}, handle.middleware...)
	middleware = append(middleware, func(context *Context, next Next) error {
		reply, err := handle.handle(node, c)
		if err != nil {
			return err
		}
		return handle.write(c, reply)
	})
	return c.doMiddleware(middleware)
}

// doMiddlewareWithHandler 执行全局中间件链，链尾自动调用 handler。
// node/params 已存在 Context 中，不需要闭包捕获，消除 make([]MiddlewareFunc) + 闭包分配。
func (c *Context) doMiddlewareWithHandler(middleware []MiddlewareFunc) error {
	total := len(middleware)
	var i int
	var next Next
	next = func() error {
		if i < total {
			mf := middleware[i]
			i++
			return mf(c, next)
		}
		if c.node == nil {
			return ErrNotFound
		}
		return c.doHandle(c.node)
	}
	return next()
}

// doMiddleware 按经典嵌套 Next 语义执行中间件链:
//   - 中间件调用 next() 返回后才是链尾完成,适合做计时/日志/recover;
//   - 中间件返回的 error 短路整条链;
//   - 中间件不调 next() 则截断后续,返回其 error(可为 nil)。
func (c *Context) doMiddleware(middleware []MiddlewareFunc) error {
	if len(middleware) == 0 {
		return nil
	}
	var i int
	var next Next
	next = func() error {
		if i >= len(middleware) {
			return nil
		}
		mf := middleware[i]
		i++
		return mf(c, next)
	}
	return next()
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
	storeInst, ok := c.stores[RequestDataTypeContext]
	if !ok {
		storeInst = values.Values{}
		c.stores[RequestDataTypeContext] = storeInst
	}
	storeInst.Set(key, val)
}

// Get 获取参数,优先路径中的params
// 其他方式直接使用c.Request...
func (c *Context) Get(key string, dataTypes ...RequestDataType) any {
	if len(dataTypes) == 0 {
		dataTypes = c.Server.RequestDataType
	}
	for _, t := range dataTypes {
		if v, ok := c.getDataFromStore(key, t); ok {
			return v
		}
	}
	return ""
}

// getDataFromStore 从存储中获取数据
func (c *Context) getDataFromStore(key string, dataType RequestDataType) (any, bool) {
	switch dataType {
	case RequestDataTypeParam:
		// 直接从 c.params 线性查找，无需创建 map
		if v, ok := c.params.Get(key); ok {
			return v, true
		}
	case RequestDataTypeQuery, RequestDataTypeBody, RequestDataTypeContext:
		// 从统一存储中获取
		store, ok := c.getOrCreateStore(dataType)
		if ok && store.Has(key) {
			return store.Get(key), true
		}
	case RequestDataTypeCookie:
		// 直接从请求中获取
		if val, err := c.Request.Cookie(key); err == nil && val.Value != "" {
			return val.Value, true
		}
	case RequestDataTypeHeader:
		// 直接从请求中获取
		if v := c.Request.Header.Get(key); v != "" {
			return v, true
		}
	}
	return "", false
}

// getOrCreateStore 获取或创建存储
func (c *Context) getOrCreateStore(dataType RequestDataType) (values.Values, bool) {
	storeInst, ok := c.stores[dataType]
	if ok {
		return storeInst, true
	}
	// 根据类型惰性创建存储
	var newStore values.Values
	switch dataType {
	case RequestDataTypeQuery:
		newStore = values.Values{}
		for k, v := range c.Request.URL.Query() {
			if len(v) > 0 {
				newStore.Set(k, v[0])
			}
		}
	case RequestDataTypeBody:
		newStore = values.Values{}
		_ = c.Bind(&newStore)
	case RequestDataTypeContext:
		newStore = values.Values{}
	}
	if newStore != nil {
		c.stores[dataType] = newStore
		return newStore, true
	}
	return nil, false
}

// GetInt 获取int类型参数
func (c *Context) GetInt(key string, dataTypes ...RequestDataType) int {
	return int(values.ParseInt64(c.Get(key, dataTypes...)))
}

// GetInt32 获取int32类型参数
func (c *Context) GetInt32(key string, dataTypes ...RequestDataType) int32 {
	return int32(values.ParseInt64(c.Get(key, dataTypes...)))
}

// GetInt64 获取int64类型参数
func (c *Context) GetInt64(key string, dataTypes ...RequestDataType) int64 {
	return values.ParseInt64(c.Get(key, dataTypes...))
}

// GetFloat 获取float64类型参数
func (c *Context) GetFloat(key string, dataTypes ...RequestDataType) float64 {
	return values.ParseFloat64(c.Get(key, dataTypes...))
}

// GetString 获取string类型参数
func (c *Context) GetString(key string, dataTypes ...RequestDataType) string {
	return values.ParseString(c.Get(key, dataTypes...))
}

// Bind 绑定JSON XML
func (c *Context) Bind(i any) (err error) {
	t := c.Request.Header.Get(HeaderContentType)
	// 忽略 "; charset=utf-8" 等参数部分
	if idx := strings.Index(t, ";"); idx >= 0 {
		t = strings.TrimSpace(t[:idx])
	}
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
		// 只有在没有错误的情况下才恢复 c.Request.Body 和缓存数据
		if err == nil {
			// 恢复 c.Request.Body，使其可重复读取
			c.Request.Body = io.NopCloser(bytes.NewReader(b.Bytes()))
			// 只有当内容大小小于等于最大缓存大小时才缓存
			if b.Len() <= int(c.Server.MaxCacheSize) {
				c.body = b.Bytes()
			}
		}
	}()
	var n int64
	// 多读一字节以区分"恰好等于上限"和"超过上限"
	reader := io.LimitReader(c.Request.Body, c.Server.MaxBodySize+1)
	n, err = b.ReadFrom(reader)
	if err != nil {
		return
	}
	if n > c.Server.MaxBodySize {
		return nil, values.Errorf(413, "request body too large")
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
		arr = append(arr, strings.Split(header, ",")...)
	}
	if header := this.Request.Header.Get(HeaderContentType); header != "" {
		arr = append(arr, strings.Split(header, ",")...)
	}

	for _, s := range arr {
		// 去掉 "; charset=utf-8" 等参数和空白
		if idx := strings.Index(s, ";"); idx >= 0 {
			s = s[:idx]
		}
		s = strings.TrimSpace(s)
		if this.accept = binder.Get(s); this.accept != nil {
			return this.accept
		}
	}
	this.accept = this.Server.Binder
	return this.accept
}
