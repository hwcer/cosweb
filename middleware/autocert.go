package middleware

import (
	"crypto/tls"
	"net/http"

	"github.com/hwcer/cosweb"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// AutoCert Let's Encrypt 自动证书管理中间件
// 自动申请、续期 HTTPS 证书，支持 HTTP-01 challenge 验证
type AutoCert struct {
	manager *autocert.Manager
}

// NewAutoCert 创建自动证书中间件
//   - cacheDir: 证书本地缓存目录（生产环境必须设置，否则每次重启重新申请）
//   - hosts: 允许签发的域名白名单（生产环境必须设置，否则任意域名都能触发签发）
func NewAutoCert(cacheDir string, hosts ...string) *AutoCert {
	m := &autocert.Manager{
		Prompt: autocert.AcceptTOS,
	}
	if cacheDir != "" {
		m.Cache = autocert.DirCache(cacheDir)
	}
	if len(hosts) > 0 {
		m.HostPolicy = autocert.HostWhitelist(hosts...)
	}
	return &AutoCert{manager: m}
}

// TLSConfig 返回配置好的 tls.Config，用于 Server.Listen 或 http.Server
func (ac *AutoCert) TLSConfig() *tls.Config {
	cfg := ac.manager.TLSConfig()
	cfg.NextProtos = append(cfg.NextProtos, acme.ALPNProto)
	return cfg
}

// Middleware HTTP-01 challenge 验证中间件
// 将此中间件注册到 HTTP（非 HTTPS）服务器，自动响应 Let's Encrypt 的验证请求
// 非验证请求重定向到 HTTPS
func (ac *AutoCert) Middleware(c *cosweb.Context, next cosweb.Next) error {
	// Let's Encrypt HTTP-01 challenge 路径: /.well-known/acme-challenge/
	if ac.manager.HTTPHandler(nil) != nil {
		handler := ac.manager.HTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 非 challenge 请求重定向到 HTTPS
			target := "https://" + r.Host + r.URL.RequestURI()
			http.Redirect(w, r, target, http.StatusMovedPermanently)
		}))
		handler.ServeHTTP(c.Response, c.Request)
		return nil
	}
	return next()
}

// RedirectHandler 返回一个 http.Handler，用于 HTTP→HTTPS 重定向 + ACME challenge 响应
// 适合独立运行在 :80 端口
func (ac *AutoCert) RedirectHandler() http.Handler {
	return ac.manager.HTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := "https://" + r.Host + r.URL.RequestURI()
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	}))
}
