package cosweb

import (
	"context"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/hwcer/logger"
)

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
	target      []*url.URL
	reverse     *httputil.ReverseProxy
	StripPrefix bool
	Transport   http.RoundTripper
	GetTarget   func(*Context, []*url.URL) *url.URL
}

func (this *Proxy) AddTarget(addr string) error {
	u, err := url.Parse(addr)
	if err != nil {
		return err
	}
	this.target = append(this.target, u)
	return nil
}

func (this *Proxy) Handle(c *Context) any {
	target := this.GetTarget(c, this.target)
	if target == nil {
		return ErrNotFound
	}
	forwardPath := c.Request.URL.Path
	if this.StripPrefix {
		forwardPath = "/" + c.GetString("*", RequestDataTypeParam)
	}
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
