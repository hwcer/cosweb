package cosweb

import (
	"github.com/hwcer/cosgo/phase"

	"reflect"

	"github.com/hwcer/cosgo/registry"
	"github.com/hwcer/logger"
)

type HandlerFunc func(*Context) any

// registry 通过registry集中注册对象
type handleCaller interface {
	Caller(node *registry.Node, c *Context) any
}

type Next func() error
type HandlerCaller func(node *registry.Node, c *Context) (any, error)
type HandlerFilter func(node *registry.Node) bool
type MiddlewareFunc func(*Context, Next) error
type HandlerSerialize func(c *Context, reply any) ([]byte, error)

type Handler struct {
	//method     []string
	caller     HandlerCaller //自定义全局消息调用
	filter     HandlerFilter
	serialize  HandlerSerialize //消息序列化封装
	middleware []MiddlewareFunc
}

// Use middleware。🔴 仅启动期调用:路由级 middleware 是裸切片,守卫读 cosgo/phase
//(公共启动阶段时钟),封板后调用只 Alert 提示并忽略——运行期 append 与请求路径
//的读是真实数据竞争
func (h *Handler) Use(middleware ...MiddlewareFunc) {
	if phase.Sealed() {
		phase.Alert("cosweb.Handler.Use")
		return
	}
	h.middleware = append(h.middleware, middleware...)
}

// SetCaller/SetFilter/SetSerialize 同 Use:仅启动期的 Handler 配置入口
func (h *Handler) SetCaller(caller HandlerCaller) {
	if phase.Sealed() {
		phase.Alert("cosweb.Handler.SetCaller")
		return
	}
	h.caller = caller
}

func (h *Handler) SetFilter(filter HandlerFilter) {
	if phase.Sealed() {
		phase.Alert("cosweb.Handler.SetFilter")
		return
	}
	h.filter = filter
}

func (h *Handler) SetSerialize(serialize HandlerSerialize) {
	if phase.Sealed() {
		phase.Alert("cosweb.Handler.SetSerialize")
		return
	}
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
			logger.Trace("handler panic: %v", e)
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
	if e, ok := reply.(*HTTPError); ok {
		return nil, e
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
	if h.serialize != nil {
		return h.serialize(c, reply)
	}
	return c.Accept().Marshal(reply)
}
