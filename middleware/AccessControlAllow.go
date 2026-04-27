package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/hwcer/cosweb"
)

/*跨域
access := middleware.NewAccessControlAllow("www.test.com","www.test1.com")
cosweb.Use(access.handle)
*/

type AccessControlAllow struct {
	expire      string
	origin      []string
	methods     []string
	headers     []string
	Credentials bool
}

func NewAccessControlAllow(origin ...string) *AccessControlAllow {
	return &AccessControlAllow{
		origin: origin,
	}
}

func (this *AccessControlAllow) Expire(second int) {
	this.expire = strconv.Itoa(second)
}
func (this *AccessControlAllow) Origin(origin ...string) {
	this.origin = append(this.origin, origin...)
}
func (this *AccessControlAllow) Methods(methods ...string) {
	this.methods = append(this.methods, methods...)
}
func (this *AccessControlAllow) Headers(headers ...string) {
	this.headers = append(this.headers, headers...)
}

func (this *AccessControlAllow) Middleware(c *cosweb.Context, next cosweb.Next) error {
	header := c.Header()

	if origin := this.matchOrigin(c.Request.Header.Get(cosweb.HeaderOrigin)); origin != "" {
		header.Set(cosweb.HeaderAccessControlAllowOrigin, origin)
		if origin != "*" {
			// 依规范,按请求返回 Origin 时需要 Vary: Origin 以保证缓存正确
			header.Add(cosweb.HeaderVary, cosweb.HeaderOrigin)
		}
	}
	if len(this.methods) > 0 {
		header.Set(cosweb.HeaderAccessControlAllowMethods, strings.Join(this.methods, ","))
	}
	if len(this.headers) > 0 {
		joined := strings.Join(this.headers, ",")
		header.Set(cosweb.HeaderAccessControlAllowHeaders, joined)
		header.Set(cosweb.HeaderAccessControlExposeHeaders, joined)
	}
	if this.Credentials {
		header.Set(cosweb.HeaderAccessControlAllowCredentials, "true")
	}
	if this.expire != "" {
		header.Set(cosweb.HeaderAccessControlMaxAge, this.expire)
	}
	//UNITY - 仅在 Unity 请求时添加这些安全头
	userAgent := c.Request.Header.Get("User-Agent")
	if strings.Contains(strings.ToLower(userAgent), "unity") {
		header.Set(cosweb.HeaderXContentTypeOptions, "nosniff")
		header.Set(cosweb.HeaderXFrameOptions, "DENY")
		header.Set(cosweb.HeaderXXSSProtection, "1; mode=block")
	}
	if c.Request.Method == http.MethodOptions {
		c.WriteHeader(http.StatusNoContent)
		return nil
	}
	return next()
}

// matchOrigin 依据规范返回单一 Origin:
//   - 配置为空 → 不返回
//   - 配置含 "*" → 若无 Credentials,返回 "*",否则回显请求 Origin
//   - 其余 → 精确匹配请求 Origin 再回显
func (this *AccessControlAllow) matchOrigin(requestOrigin string) string {
	if len(this.origin) == 0 {
		return ""
	}
	for _, o := range this.origin {
		if o == "*" {
			if this.Credentials {
				return requestOrigin
			}
			return "*"
		}
		if o == requestOrigin {
			return o
		}
	}
	return ""
}
