package cosweb

import (
	"fmt"
	"github.com/hwcer/registry"
	"path"
	"reflect"
	"strings"
)

// 通过registry集中注册对象

type RegistryCaller func(c *Context, pr reflect.Value, fn reflect.Value) (interface{}, error)
type RegistrySerialize func(ctx *Context, reply interface{}) error
type RegistryInterface interface {
	Caller(c *Context, fn reflect.Value) interface{}
}

type RegistryHandler struct {
	Caller     RegistryCaller    //自定义消息调用方式
	Serialize  RegistrySerialize //消息序列化封装
	Middleware []MiddlewareFunc  //中间件
}

func (this *RegistryHandler) Use(src interface{}) {
	if v, ok := src.(RegistryCaller); ok {
		this.Caller = v
	}
	if v, ok := src.(RegistrySerialize); ok {
		this.Serialize = v
	}
	if v, ok := src.(MiddlewareFunc); ok {
		this.Middleware = append(this.Middleware, v)
	}
}

//NewRegistry 创建新的路由组
// prefix路由前缀
func NewRegistry(prefix string, opts *registry.Options) *Registry {
	r := &Registry{}
	if opts == nil {
		opts = registry.NewOptions()
	}
	if opts.Filter == nil {
		opts.Filter = r.filter
	}
	r.prefix = opts.Clean(prefix)
	r.handler = make(map[string]*RegistryHandler)
	r.Registry = registry.New(opts)
	return r
}

type Registry struct {
	*registry.Registry
	prefix    string
	handler   map[string]*RegistryHandler
	Caller    RegistryCaller    //自定义全局消息调用
	Serialize RegistrySerialize //消息序列化封装
}

func (r *Registry) filter(s *registry.Service, pr, fn reflect.Value) bool {
	if !pr.IsValid() {
		_, ok := fn.Interface().(func(*Context) interface{})
		return ok
	}
	t := fn.Type()
	if t.NumIn() != 2 {
		return false
	}
	if t.NumOut() != 1 {
		return false
	}
	return true
}

//handle cosweb入口
func (r *Registry) handle(c *Context, next Next) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("%v", e)
			//logger.Error("%v", err)
		}
	}()

	if c.Request.URL.Path == "" || strings.Contains(c.Request.URL.Path, ".") {
		return next()
	}
	n := len(c.route)
	route := make([]string, n, n)
	copy(route, c.route)
	route[0] = ""
	route[n-1] = c.params["*"]
	urlPath := r.Clean(strings.Join(route, "/"))
	if r.prefix != "" {
		urlPath = strings.TrimPrefix(urlPath, r.prefix)
	}
	service, ok := r.Match(urlPath)
	if !ok {
		return next()
	}
	pr, fn, ok := service.Match(urlPath)
	if !ok {
		return next()
	}
	name := service.Name()
	handler := r.handler[name]
	if handler != nil && len(handler.Middleware) > 0 {
		if err, ok = c.doMiddleware(handler.Middleware); err != nil || !ok {
			return
		}
	}

	var reply interface{}
	if handler != nil && handler.Caller != nil {
		reply, err = handler.Caller(c, pr, fn)
	} else if r.Caller != nil {
		reply, err = r.Caller(c, pr, fn)
	} else {
		reply, err = r.caller(c, pr, fn)
	}
	if err != nil {
		return
	}

	if handler != nil && handler.Serialize != nil {
		return handler.Serialize(c, reply)
	} else if r.Serialize != nil {
		return r.Serialize(c, reply)
	} else {
		return c.JSON(reply)
	}
}

func (r *Registry) caller(c *Context, pr, fn reflect.Value) (reply interface{}, err error) {
	if !pr.IsValid() {
		f, _ := fn.Interface().(func(c *Context) interface{})
		reply = f(c)
	} else if s, ok := pr.Interface().(RegistryInterface); ok {
		reply = s.Caller(c, fn)
	} else {
		ret := fn.Call([]reflect.Value{pr, reflect.ValueOf(c)})
		reply = ret[0].Interface()
	}
	return
}

func (r *Registry) Service(name string, middleware ...interface{}) *registry.Service {
	service := r.Registry.Service(name)
	if len(middleware) > 0 {
		handler := &RegistryHandler{}
		for _, m := range middleware {
			handler.Use(m)
		}
		r.handler[service.Name()] = handler
	}
	return service
}

//Handle 注册服务器
func (r *Registry) Handle(s *Server, method ...string) {
	for _, service := range r.Registry.Services() {
		route := path.Join(r.prefix, service.Name(), "*")
		s.Register(route, r.handle, method...)
	}
}
