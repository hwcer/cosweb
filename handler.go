package cosweb

import (
	"github.com/hwcer/cosgo/message"
	"github.com/hwcer/logger"
	"github.com/hwcer/registry"
	"reflect"
	"runtime/debug"
	"strings"
)

// registry 通过registry集中注册对象
type handleCaller interface {
	Caller(node *registry.Node, c *Context) interface{}
}

type HandlerCaller func(node *registry.Node, c *Context) (interface{}, error)

type HandlerSerialize func(c *Context, reply interface{}) error

type Handler struct {
	method     []string
	caller     HandlerCaller    //自定义全局消息调用
	serialize  HandlerSerialize //消息序列化封装
	middleware []MiddlewareFunc
}

func (h *Handler) Use(src interface{}) {
	if v, ok := src.(HandlerCaller); ok {
		h.caller = v
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

// handle cosweb入口
func (h *Handler) handle(c *Context, next Next) (err error) {
	defer func() {
		if e := recover(); e != nil {
			if v := recover(); v != nil {
				logger.Info("cosweb server recover error:%v\n%v", v, string(debug.Stack()))
			}
		}
	}()
	if c.Request.URL.Path == "" || strings.Contains(c.Request.URL.Path, ".") {
		return next()
	}
	r := c.engine.Registry
	urlPath := r.Clean(strings.Join(c.route[1:], "/"))
	node, ok := r.Match(urlPath)
	if !ok {
		return next()
	}
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
	reply, err := handler.Caller(c, node)
	if err != nil {
		return
	}
	return handler.Serialize(c, reply)
}

func (h *Handler) Caller(c *Context, node *registry.Node) (reply interface{}, err error) {
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
func (this *Handler) Serialize(c *Context, reply interface{}) error {
	if this.serialize != nil {
		return this.serialize(c, reply)
	}
	return c.JSON(message.Parse(reply))
}
