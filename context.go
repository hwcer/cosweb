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
	c.Response = &Response{ResponseWriter: w, canWrite: true}
	// 清空 stores
	for k := range c.stores {
		delete(c.stores, k)
	}
}

// 释放资源,准备进入缓存池
func (c *Context) release() {
	c.body = nil
	c.accept = nil
	// 清空 stores
	for k := range c.stores {
		delete(c.stores, k)
	}
	c.Request = nil
	c.Response = nil
	c.Session.Release()
}

func (c *Context) doHandle(node *registry.Node, params map[string]string) error {
	handle, ok := node.Handler().(*Handler)
	if !ok {
		return ErrHandlerError
	}
	// 创建并设置路径参数
	pathValues := values.Values{}
	for k, v := range params {
		pathValues.Set(k, v)
	}
	c.stores[RequestDataTypeParam] = pathValues

	if err := c.doMiddleware(handle.middleware); err != nil {
		return err
	}
	reply, err := handle.handle(node, c)
	if err != nil {
		return err
	}
	return handle.write(c, reply)
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
	storeInst, ok := c.stores[RequestDataTypeContext]
	if !ok {
		storeInst = values.Values{}
		c.stores[RequestDataTypeContext] = storeInst
	}
	storeInst.Set(key, val)
}

// Get 获取参数,优先路径中的params
// 其他方式直接使用c.Request...
func (c *Context) Get(key string, dataTypes ...RequestDataType) interface{} {
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
	case RequestDataTypeParam, RequestDataTypeQuery, RequestDataTypeBody, RequestDataTypeContext:
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
	// 根据类型创建存储
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
func (c *Context) GetInt(key string, dataTypes ...RequestDataType) (r int) {
	return int(c.GetInt64(key, dataTypes...))
}

// GetInt64 获取int64类型参数
func (c *Context) GetInt64(key string, dataTypes ...RequestDataType) (r int64) {
	if len(dataTypes) == 0 {
		dataTypes = c.Server.RequestDataType
	}
	for _, t := range dataTypes {
		switch t {
		case RequestDataTypeParam, RequestDataTypeQuery, RequestDataTypeBody, RequestDataTypeContext:
			// 直接从values.Values中获取
			if store, ok := c.getOrCreateStore(t); ok {
				if store.Has(key) {
					return store.GetInt64(key)
				}
			}
		case RequestDataTypeCookie:
			// 直接从请求中获取
			if val, err := c.Request.Cookie(key); err == nil && val.Value != "" {
				return values.ParseInt64(val.Value)
			}
		case RequestDataTypeHeader:
			// 直接从请求中获取
			if v := c.Request.Header.Get(key); v != "" {
				return values.ParseInt64(v)
			}
		}
	}
	return 0
}

// GetInt32 获取int32类型参数
func (c *Context) GetInt32(key string, dataTypes ...RequestDataType) (r int32) {
	return int32(c.GetInt64(key, dataTypes...))
}

// GetFloat 获取float64类型参数
func (c *Context) GetFloat(key string, dataTypes ...RequestDataType) (r float64) {
	if len(dataTypes) == 0 {
		dataTypes = c.Server.RequestDataType
	}
	for _, t := range dataTypes {
		switch t {
		case RequestDataTypeParam, RequestDataTypeQuery, RequestDataTypeBody, RequestDataTypeContext:
			// 直接从values.Values中获取
			if store, ok := c.getOrCreateStore(t); ok {
				if store.Has(key) {
					return store.GetFloat64(key)
				}
			}
		case RequestDataTypeCookie:
			// 直接从请求中获取
			if val, err := c.Request.Cookie(key); err == nil && val.Value != "" {
				return values.ParseFloat64(val.Value)
			}
		case RequestDataTypeHeader:
			// 直接从请求中获取
			if v := c.Request.Header.Get(key); v != "" {
				return values.ParseFloat64(v)
			}
		}
	}
	return 0
}

// GetString 获取string类型参数
func (c *Context) GetString(key string, dataTypes ...RequestDataType) (r string) {
	if len(dataTypes) == 0 {
		dataTypes = c.Server.RequestDataType
	}
	for _, t := range dataTypes {
		switch t {
		case RequestDataTypeParam, RequestDataTypeQuery, RequestDataTypeBody, RequestDataTypeContext:
			// 直接从values.Values中获取
			if store, ok := c.getOrCreateStore(t); ok {
				if store.Has(key) {
					return store.GetString(key)
				}
			}
		case RequestDataTypeCookie:
			// 直接从请求中获取
			if val, err := c.Request.Cookie(key); err == nil && val.Value != "" {
				return val.Value
			}
		case RequestDataTypeHeader:
			// 直接从请求中获取
			if v := c.Request.Header.Get(key); v != "" {
				return v
			}
		}
	}
	return ""
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
		// 只有当内容大小小于等于最大缓存大小时才缓存
		if b.Len() <= int(c.Server.MaxCacheSize) {
			c.body = b.Bytes()
		}
	}()
	var n int64
	// 使用 io.LimitReader 限制读取大小
	reader := io.LimitReader(c.Request.Body, c.Server.MaxBodySize)
	n, err = b.ReadFrom(reader)
	if err != nil {
		return
	}
	// 检查是否超过大小限制
	if n >= c.Server.MaxBodySize {
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
