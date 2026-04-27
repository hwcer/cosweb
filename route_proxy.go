package cosweb

import (
	"context"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/hwcer/logger"
)

// NewProxy 创建一个反向代理。参数为可选的上游地址列表。
func NewProxy(address ...string) *Proxy {
	p := &Proxy{GetTarget: defaultProxyGetTarget}
	for _, addr := range address {
		_ = p.AddTarget(addr)
	}
	p.reverse = &httputil.ReverseProxy{
		Rewrite:      p.rewrite,
		ErrorHandler: p.errorHandler,
	}
	return p
}

type Proxy struct {
	prefix      string
	target      []*url.URL
	reverse     *httputil.ReverseProxy
	methods     map[string]bool
	StripPrefix bool                                  // 转发时是否剥离前缀，默认 false（保留原始路径）
	Transport   http.RoundTripper                     //自定义 RoundTripper,nil 时使用 http.DefaultTransport
	GetTarget   func(*Context, []*url.URL) *url.URL   //负载均衡钩子
}

// AddTarget 追加上游地址。
func (this *Proxy) AddTarget(addr string) error {
	u, err := url.Parse(addr)
	if err != nil {
		return err
	}
	this.target = append(this.target, u)
	return nil
}

// Middleware 全局中间件：匹配前缀的请求转发到上游，不匹配调 next()
func (this *Proxy) Middleware(c *Context, next Next) error {
	if len(this.methods) > 0 && !this.methods[c.Request.Method] {
		return next()
	}
	path := c.Request.URL.Path
	if this.prefix != "/" && !strings.HasPrefix(path, this.prefix+"/") && path != this.prefix {
		return next()
	}
	target := this.GetTarget(c, this.target)
	if target == nil {
		return next()
	}

	// 计算转发路径
	forwardPath := path
	if this.StripPrefix {
		forwardPath = strings.TrimPrefix(path, this.prefix)
		if !strings.HasPrefix(forwardPath, "/") {
			forwardPath = "/" + forwardPath
		}
	}

	// 存转发路径到 request context，供 rewrite 使用
	c.Request = c.Request.WithContext(withProxyPath(c.Request.Context(), forwardPath))

	rp := this.reverse
	if this.Transport != nil {
		cp := *rp
		cp.Transport = this.Transport
		rp = &cp
	}
	rp.ServeHTTP(c.Response, c.Request)
	c.Response.written = true
	return nil
}

type proxyPathKey struct{}

func withProxyPath(parent context.Context, path string) context.Context {
	return context.WithValue(parent, proxyPathKey{}, path)
}

func (this *Proxy) rewrite(pr *httputil.ProxyRequest) {
	target := this.GetTarget(nil, this.target)
	if target == nil {
		return
	}

	forwardPath, _ := pr.In.Context().Value(proxyPathKey{}).(string)
	if forwardPath == "" {
		forwardPath = pr.In.URL.Path
	}

	dst := *target
	dst.Path = forwardPath
	dst.RawQuery = pr.In.URL.RawQuery
	dst.Fragment = pr.In.URL.Fragment
	if pr.In.URL.User != nil {
		dst.User = pr.In.URL.User
	}
	pr.Out.URL = &dst
	pr.Out.Host = dst.Host
	pr.SetXForwarded()
}

func (this *Proxy) errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	logger.Alert("proxy error: %v", err)
	w.WriteHeader(http.StatusBadGateway)
}

func defaultProxyGetTarget(_ *Context, address []*url.URL) *url.URL {
	switch len(address) {
	case 0:
		return nil
	case 1:
		return address[0]
	default:
		return address[rand.Intn(len(address))]
	}
}
