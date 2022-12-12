package cosweb

import (
	"fmt"
	"github.com/hwcer/cosgo/logger"
	"github.com/hwcer/cosgo/registry"
	"github.com/hwcer/cosgo/values"
	"reflect"
	"runtime/debug"
)

// registry 通过registry集中注册对象
type handleCaller interface {
	Caller(node *registry.Node, c *Context) interface{}
}

type HandlerCaller func(node *registry.Node, c *Context) (interface{}, error)
type HandlerFilter func(node *registry.Node) bool
type HandlerSerialize func(c *Context, reply interface{}) (interface{}, error)

type Handler struct {
	method     []string
	caller     HandlerCaller //自定义全局消息调用
	filter     HandlerFilter
	serialize  HandlerSerialize //消息序列化封装
	middleware []MiddlewareFunc
}

func (h *Handler) Use(src interface{}) {
	if v, ok := src.(HandlerCaller); ok {
		h.caller = v
	}
	if v, ok := src.(HandlerFilter); ok {
		h.filter = v
	}
	if v, ok := src.(HandlerSerialize); ok {
		h.serialize = v
	}
	if v, ok := src.(MiddlewareFunc); ok {
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
		_, ok := node.Method().(func(*Context) interface{})
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
	return func(c *Context, next Next) error {
		return h.handle(c, node)
	}
}

// handle cosweb入口
func (h *Handler) handle(c *Context, node *registry.Node) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("%v", e)
			logger.Info("cosweb server recover error:%v\n%v", e, string(debug.Stack()))
		}
	}()
	service := node.Service()
	handler, ok := service.Handler.(*Handler)
	if !ok {
		return ErrHandlerError
	}
	if len(handler.middleware) > 0 {
		if err, ok = c.doMiddleware(handler.middleware); err != nil || !ok {
			return
		}
	}
	reply, err := handler.Caller(node, c)
	if err != nil {
		return
	}
	return handler.Serialize(c, reply)
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
	if this.serialize != nil {
		reply, err = this.serialize(c, reply)
	}
	if err != nil || !c.Writable() {
		return err
	}
	var ok bool
	var data []byte
	if data, ok = reply.([]byte); !ok {
		data, err = c.Binder.Marshal(values.NewMessage(reply))
	}
	if err != nil {
		return err
	} else {
		return c.Bytes(ContentType(c.Binder.String()), data)
	}
}
