package cosweb

import (
	"reflect"
	"runtime/debug"

	"github.com/hwcer/cosgo/registry"
	"github.com/hwcer/logger"
)

type HandlerFunc func(*Context) any

// registry 通过registry集中注册对象
type handleCaller interface {
	Caller(node *registry.Node, c *Context) any
}

// type Next func() error
type HandlerCaller func(node *registry.Node, c *Context) (interface{}, error)
type HandlerFilter func(node *registry.Node) bool
type MiddlewareFunc func(*Context) bool
type HandlerSerialize func(c *Context, reply interface{}) ([]byte, error)

type Handler struct {
	//method     []string
	caller     HandlerCaller //自定义全局消息调用
	filter     HandlerFilter
	serialize  HandlerSerialize //消息序列化封装
	middleware []MiddlewareFunc
}

// Use middleware
func (h *Handler) Use(middleware ...func(*Context) bool) {
	for _, m := range middleware {
		h.middleware = append(h.middleware, m)
	}
}

func (h *Handler) SetCaller(caller func(node *registry.Node, c *Context) (interface{}, error)) {
	h.caller = caller
}

func (h *Handler) SetFilter(filter func(node *registry.Node) bool) {
	h.filter = filter
}

func (h *Handler) SetSerialize(serialize func(c *Context, reply interface{}) ([]byte, error)) {
	h.serialize = serialize
}

func (h *Handler) Filter(node *registry.Node) bool {
	if h.filter != nil {
		return h.filter(node)
	}
	if node.IsFunc() {
		i := node.Method()
		_, ok := i.(func(*Context) any)
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

func (h *Handler) handle(node *registry.Node, c *Context) (reply any, err error) {
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
		f, _ := node.Method().(func(*Context) any)
		reply = f(c)
	} else if s, ok := node.Binder().(handleCaller); ok {
		reply = s.Caller(node, c)
	} else {
		ret := node.Call(c)
		reply = ret[0].Interface()
	}
	return
}

func (h *Handler) write(c *Context, reply any) (err error) {
	if !c.Response.CanWrite() {
		return nil
	}
	b := c.Accept()
	switch v := reply.(type) {
	case []byte:
		return c.Bytes(ContentType(b.String()), v)
	case *[]byte:
		return c.Bytes(ContentType(b.String()), *v)
	default:
		var data []byte
		if h.serialize != nil {
			data, err = h.serialize(c, reply)
		} else {
			data, err = h.defaultSerialize(c, reply)
		}
		if err == nil {
			return c.Bytes(ContentType(b.String()), data)
		} else {
			return err
		}
	}
}

func (h *Handler) defaultSerialize(c *Context, reply any) ([]byte, error) {
	b := c.Accept()
	return b.Marshal(reply)
}
