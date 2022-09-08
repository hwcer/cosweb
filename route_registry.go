package cosweb

import (
	"fmt"
	"github.com/hwcer/registry"
	"path"
	"strings"
)

// registry 通过registry集中注册对象
type registryInterface interface {
	Caller(c *Context, node *registry.Node) interface{}
}
type RegistryCaller func(c *Context, node *registry.Node) (interface{}, error)
type RegistrySerialize func(ctx *Context, reply interface{}) error

//type RegistryMiddleware func(*Context, Next) error

type registryCallerHandle interface {
	Caller(c *Context, node *registry.Node) (interface{}, error)
}
type registrySerializeHandle interface {
	Serialize(ctx *Context, reply interface{}) error
}
type registryMiddlewareHandle interface {
	Middleware(*Context, Next) error
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

	if v, ok := src.(registryCallerHandle); ok {
		this.Caller = v.Caller
	}
	if v, ok := src.(registrySerializeHandle); ok {
		this.Serialize = v.Serialize
	}
	if v, ok := src.(registryMiddlewareHandle); ok {
		this.Middleware = append(this.Middleware, v.Middleware)
	}

}

// NewRegistry 创建新的路由组
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

func (r *Registry) filter(s *registry.Service, node *registry.Node) bool {
	if node.IsFunc() {
		_, ok := node.Method().(func(*Context) interface{})
		return ok
	}
	fn := node.Value()
	t := fn.Type()
	if t.NumIn() != 2 {
		return false
	}
	if t.NumOut() != 1 {
		return false
	}
	return true
}

// handle cosweb入口
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
	urlPath := r.Clean(strings.Join(c.route[1:], "/"))
	if r.prefix != "" {
		urlPath = strings.TrimPrefix(urlPath, r.prefix)
	}

	service, ok := r.Match(urlPath)
	if !ok {
		return next()
	}
	node, ok := service.Match(urlPath)
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
		reply, err = handler.Caller(c, node)
	} else if r.Caller != nil {
		reply, err = r.Caller(c, node)
	} else {
		reply, err = r.caller(c, node)
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

func (r *Registry) caller(c *Context, node *registry.Node) (reply interface{}, err error) {
	if node.IsFunc() {
		f, _ := node.Method().(func(c *Context) interface{})
		reply = f(c)
	} else if s, ok := node.Binder().(registryInterface); ok {
		reply = s.Caller(c, node)
	} else {
		ret := node.Call(c)
		reply = ret[0].Interface()
	}
	return
}

func (r *Registry) Service(name string, middleware ...interface{}) (service *registry.Service) {
	service = r.Registry.Service(name)
	if len(middleware) == 0 {
		return
	}
	handler, ok := r.handler[service.Name()]
	if !ok {
		handler = &RegistryHandler{}
		r.handler[service.Name()] = handler
	}
	for _, m := range middleware {
		handler.Use(m)
	}
	return
}

// Handle 注册服务器
func (r *Registry) Handle(s *Server, method ...string) {
	for _, service := range r.Registry.Services() {
		for _, v := range service.Paths() {
			route := path.Join(r.prefix, service.Name(), v)
			s.Register(route, r.handle, method...)
		}
	}
}
