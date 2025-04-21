package cosweb

import (
	"github.com/hwcer/cosgo/registry"
	"github.com/hwcer/cosgo/values"
	"github.com/hwcer/logger"
	"reflect"
	"runtime/debug"
)

// registry 通过registry集中注册对象
type handleCaller interface {
	Caller(node *registry.Node, c *Context) any
}
type Next func() error
type HandlerCaller func(node *registry.Node, c *Context) (interface{}, error)
type HandlerFilter func(node *registry.Node) bool
type MiddlewareFunc func(*Context, Next) error
type HandlerSerialize func(c *Context, reply interface{}) (interface{}, error)

type Handler struct {
	method     []string
	caller     HandlerCaller //自定义全局消息调用
	filter     HandlerFilter
	serialize  HandlerSerialize //消息序列化封装
	middleware []MiddlewareFunc
}

func (h *Handler) Use(src interface{}) {
	switch v := src.(type) {
	case HandlerCaller:
		h.caller = v
	case HandlerFilter:
		h.filter = v
	case HandlerSerialize:
		h.serialize = v
	case MiddlewareFunc:
		h.middleware = append(h.middleware, v)
	case []string:
		h.method = append(h.method, v...)
	default:
		h.useFromFunc(src)
	}
}
func (h *Handler) useFromFunc(src any) {
	if v, ok := src.(func(node *registry.Node, c *Context) (interface{}, error)); ok {
		h.caller = v
	}
	if v, ok := src.(HandlerCaller); ok {
		h.caller = v
	}
	if v, ok := src.(func(node *registry.Node) bool); ok {
		h.filter = v
	}
	if v, ok := src.(func(c *Context, reply interface{}) (interface{}, error)); ok {
		h.serialize = v
	}
	if v, ok := src.(func(*Context, Next) error); ok {
		h.middleware = append(h.middleware, v)
	}
	if v, ok := src.([]string); ok {
		h.method = append(h.method, v...)
	}
}

func (h *Handler) Filter(node *registry.Node) bool {
	if h.filter != nil {
		return h.filter(node)
	}
	if node.IsFunc() {
		_, ok := node.Method().(func(*Context) any)
		return ok
	} else if node.IsMethod() {
		t := node.Value().Type()
		if t.NumIn() != 2 || t.NumOut() != 1 {
			return false
		}
		return true
	} else {
		if _, ok := node.Binder().(handleCaller); !ok {
			v := reflect.Indirect(reflect.ValueOf(node.Binder()))
			logger.Debug("[%v]未正确实现Caller方法,会影响程序性能", v.Type().String())
		}
		return true
	}
}

// closure 闭包绑定Node和route
func (h *Handler) closure(node *registry.Node) HandlerFunc {
	return func(c *Context) error {
		return h.handle(node, c)
	}
}

// handle cosweb入口
func (h *Handler) handle(node *registry.Node, c *Context) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = ErrInternalServerError
			logger.Trace("%v\n%v", e, string(debug.Stack()))
		}
	}()

	if !c.doMiddleware(h.middleware) {
		return nil
	}

	reply, err := h.Caller(node, c)
	if err != nil {
		return
	}
	return h.Serialize(c, reply)
}

func (h *Handler) Caller(node *registry.Node, c *Context) (reply interface{}, err error) {
	if h.caller != nil {
		return h.caller(node, c)
	}
	if node.IsFunc() {
		f, _ := node.Method().(func(c *Context) interface{})
		reply = f(c)
	} else if s, ok := node.Binder().(handleCaller); ok {
		reply = s.Caller(node, c)
	} else {
		ret := node.Call(c)
		reply = ret[0].Interface()
	}
	return
}
func (this *Handler) Serialize(c *Context, reply interface{}) (err error) {
	if !c.Writable() {
		return nil
	}
	if this.serialize != nil {
		reply, err = this.serialize(c, reply)
	}
	if err != nil || !c.Writable() {
		return err
	}
	var ok bool
	var data []byte
	if data, ok = reply.([]byte); !ok {
		data, err = c.Binder.Marshal(values.Parse(reply))
	}
	if err != nil {
		return err
	} else {
		return c.Bytes(ContentType(c.Binder.String()), data)
	}
}
