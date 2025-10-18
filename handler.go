package cosweb

import (
	"reflect"
	"runtime/debug"

	"github.com/hwcer/cosgo/registry"
	"github.com/hwcer/cosgo/values"
	"github.com/hwcer/logger"
)

// registry 通过registry集中注册对象
type handleCaller interface {
	Caller(node *registry.Node, c *Context) any
}
type Next func() error
type HandlerCaller func(node *registry.Node, c *Context) (interface{}, error)
type HandlerFilter func(node *registry.Node) bool
type MiddlewareFunc func(*Context) bool
type HandlerSerialize func(c *Context, reply interface{}) ([]byte, error)

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
	if v, ok := src.(func(c *Context, reply interface{}) ([]byte, error)); ok {
		h.serialize = v
	}
	if v, ok := src.(func(*Context) bool); ok {
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
		i := node.Method()
		_, ok := i.(HandlerFunc)
		return ok
	} else if node.IsMethod() {
		t := node.Value().Type()
		if t.NumIn() != 2 || t.NumOut() != 1 {
			return false
		}
		return true
	} else if node.IsStruct() {
		if _, ok := node.Binder().(handleCaller); !ok {
			v := reflect.Indirect(reflect.ValueOf(node.Binder()))
			logger.Debug("[%v]未正确实现Caller方法,会影响程序性能", v.Type().String())
		}
		return true
	}
	return true
}

func (h *Handler) Caller(node *registry.Node, c *Context) (reply any, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = ErrInternalServerError
			logger.Trace("%v\n%v", e, string(debug.Stack()))
		}
	}()
	if h.caller != nil {
		return h.caller(node, c)
	}
	if node.IsFunc() {
		f, _ := node.Method().(HandlerFunc)
		reply = f(c)
	} else if s, ok := node.Binder().(handleCaller); ok {
		reply = s.Caller(node, c)
	} else {
		ret := node.Call(c)
		reply = ret[0].Interface()
	}
	return
}

func (this *Handler) defaultSerialize(c *Context, reply any) ([]byte, error) {
	if this.serialize != nil {
		return this.serialize(c, reply)
	}
	b := c.Accept()
	v := values.Parse(reply)
	return b.Marshal(v)
}
func (this *Handler) Serialize(c *Context, reply any) (err error) {
	b := c.Accept()
	switch v := reply.(type) {
	case []byte:
		return c.Bytes(ContentType(b.String()), v)
	case *[]byte:
		return c.Bytes(ContentType(b.String()), *v)
	default:
		var data []byte
		if data, err = this.defaultSerialize(c, reply); err != nil {
			return err
		} else {
			return c.Bytes(ContentType(b.String()), data)
		}
	}
}
