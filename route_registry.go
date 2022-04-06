package cosweb

import (
	"fmt"
	"github.com/hwcer/registry"
	"path"
	"reflect"
	"strings"
)

// 通过registry集中注册对象

type RegistryCaller interface {
	Caller(c *Context, fn reflect.Value) interface{}
}

type RegistrySerialize func(ctx *Context, reply interface{}) error

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
	r.Registry = registry.New(opts)
	r.middleware = make(map[string][]MiddlewareFunc)
	r.prefix = opts.Clean(prefix)
	return r
}

type Registry struct {
	*registry.Registry
	prefix     string
	Caller     func(c *Context, pr reflect.Value, fn reflect.Value) (interface{}, error) //自定义全局消息调用
	Serialize  RegistrySerialize                                                         //消息序列化封装
	middleware map[string][]MiddlewareFunc
}

func (r *Registry) filter(pr, fn reflect.Value) bool {
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
	if c.Request.URL.Path == "" || strings.Contains(c.Request.URL.Path, ".") {
		return next()
	}
	urlPath := r.Clean(c.Request.URL.Path)
	if r.prefix != "" {
		urlPath = strings.TrimPrefix(urlPath, r.prefix)
	}

	route, ok := r.Match(urlPath)
	if !ok {
		return next()
	}

	pr, fn, ok := route.Match(urlPath)
	if !ok {
		return next()
	}
	name := route.Name()
	if err, ok = c.doMiddleware(r.middleware[name]); err != nil || !ok {
		return
	}

	var reply interface{}
	reply, err = r.caller(c, pr, fn)

	if err != nil {
		return
	}
	if r.Serialize != nil {
		return r.Serialize(c, reply)
	} else {
		return c.JSON(reply)
	}
}

func (r *Registry) caller(c *Context, pr, fn reflect.Value) (reply interface{}, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("%v", e)
			//logger.Error("%v", err)
		}
	}()
	if r.Caller != nil {
		return r.Caller(c, pr, fn)
	}
	if !pr.IsValid() {
		f, _ := fn.Interface().(func(c *Context) interface{})
		reply = f(c)
	} else if s, ok := pr.Interface().(RegistryCaller); ok {
		reply = s.Caller(c, fn)
	} else {
		ret := fn.Call([]reflect.Value{pr, reflect.ValueOf(c)})
		reply = ret[0].Interface()
	}
	return
}

func (r *Registry) Service(name string, middleware ...MiddlewareFunc) *registry.Service {
	route := r.Registry.Service(name)
	if len(middleware) > 0 {
		s := route.Name()
		r.middleware[s] = append(r.middleware[s], middleware...)
	}
	return route
}

//Handle 注册服务器
func (r *Registry) Handle(s *Server, method ...string) {
	r.Range(func(name string, _ *registry.Service) bool {
		route := path.Join(r.prefix, name, "*")
		s.Register(route, r.handle, method...)
		return true
	})
}
