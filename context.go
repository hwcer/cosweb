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

type dispatch struct {
	index   int
	funcs   []MiddlewareFunc
	handler *Handler
	nexts   []registry.SearchResult
}

func (mid *dispatch) Release() {
	mid.funcs = nil
	mid.handler = nil
	mid.nexts = nil
}

// Context API上下文.
type Context struct {
	body       []byte
	accept     binder.Binder                     //客户端接受的序列化方式
	stores     map[RequestDataType]values.Values // 统一存储所有参数
	node       *registry.Node                    // 当前匹配的路由节点（避免闭包分配）
	params     registry.Params                   // 当前路径参数
	dp         dispatch
	dispatchFn Next     // 缓存 c.doDispatch 方法值，避免每次传递时分配
	response   Response // 内嵌值，避免每次请求堆分配
	Server     *Server
	Session    *session.Session
	Request    *http.Request
	Response   *Response
}

// NewContext returns a Context instance.
func NewContext(s *Server) *Context {
	c := &Context{
		Server: s,
		stores: make(map[RequestDataType]values.Values),
	}
	c.Session = session.New()
	c.dispatchFn = c.doDispatch
	return c
}

func (c *Context) reset(w http.ResponseWriter, r *http.Request) {
	c.Request = r
	c.response.ResponseWriter = w
	c.response.status = 0
	c.response.written = false
	c.response.hijacked = false
	c.Response = &c.response
	c.node = nil
	c.params = nil
	c.dp = dispatch{}
	clear(c.stores)
}

// 释放资源,准备进入缓存池
func (c *Context) release() {
	c.body = nil
	c.accept = nil
	clear(c.stores)
	c.Request = nil
	c.response.ResponseWriter = nil
	c.Response = nil
	c.dp.Release()
	c.Session.Release()
}

func (c *Context) Next() error {
	if c.dp.nexts == nil {
		all := c.Server.Registry.SearchAll(c.Request.Method, c.Request.URL.Path)
		if len(all) > 1 {
			c.dp.nexts = all[1:]
		}
	}
	if len(c.dp.nexts) == 0 {
		return ErrNotFound
	}
	r := c.dp.nexts[0]
	c.dp.nexts = c.dp.nexts[1:]
	c.node = r.Node
	c.params = r.Params
	handle, ok := r.Node.Handler().(*Handler)
	if !ok {
		return ErrHandlerError
	}
	if handle != c.dp.handler && len(handle.middleware) > 0 {
		funcs := make([]MiddlewareFunc, c.dp.index+len(handle.middleware))
		copy(funcs, c.dp.funcs[:c.dp.index])
		copy(funcs[c.dp.index:], handle.middleware)
		c.dp.funcs = funcs
	}
	c.dp.handler = handle
	return c.doDispatch()
}

func (c *Context) doDispatch() error {
	if c.dp.index < len(c.dp.funcs) {
		mf := c.dp.funcs[c.dp.index]
		c.dp.index++
		return mf(c, c.dispatchFn)
	}
	if c.dp.handler == nil || c.node == nil {
		return ErrNotFound
	}
	reply, err := c.dp.handler.handle(c.node, c)
	if err != nil {
		return err
	}
	return c.dp.handler.write(c, reply)
}

// IsWebSocket 判断是否WebSocket
func (c *Context) IsWebSocket() bool {
	return strings.EqualFold(c.Request.Header.Get(HeaderUpgrade), "websocket")
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
	if ip := c.Request.Header.Get(HeaderXForwardedFor); ip != "" {
		if i := strings.IndexByte(ip, ','); i >= 0 {
			return ip[:i]
		}
		return ip
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
	if idx := strings.IndexByte(t, ';'); idx >= 0 {
		t = strings.TrimSpace(t[:idx])
	}
	encoder := binder.Get(t)
	if encoder == nil {
		return values.Errorf(0, "unknown content type: %s", t)
	}
	// 已缓存时直接使用，跳过 Buffer() 的 bytes.Buffer 结构体分配
	if c.body != nil {
		if len(c.body) > 0 {
			return encoder.Unmarshal(c.body, i)
		}
		return nil
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
	initCap := 256
	if cl := c.Request.ContentLength; cl > 0 && cl <= c.Server.MaxBodySize {
		initCap = int(cl)
	}
	b = bytes.NewBuffer(make([]byte, 0, initCap))
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

func (c *Context) Accept() binder.Binder {
	if c.accept != nil {
		return c.accept
	}
	// 栈数组 + 手动切割，避免 strings.Split 的 []string 堆分配
	for _, header := range [2]string{
		c.Request.Header.Get(HeaderAccept),
		c.Request.Header.Get(HeaderContentType),
	} {
		for header != "" {
			var s string
			if i := strings.IndexByte(header, ','); i >= 0 {
				s, header = header[:i], header[i+1:]
			} else {
				s, header = header, ""
			}
			if i := strings.IndexByte(s, ';'); i >= 0 {
				s = s[:i]
			}
			s = strings.TrimSpace(s)
			if c.Server.AcceptIgnore[s] {
				continue
			}
			if c.accept = binder.Get(s); c.accept != nil {
				return c.accept
			}
		}
	}
	c.accept = c.Server.Binder
	return c.accept
}
