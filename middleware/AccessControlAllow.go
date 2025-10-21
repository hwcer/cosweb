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

func NewAccessControlAllowHandle(origin ...string) cosweb.MiddlewareFunc {
	aca := NewAccessControlAllow(origin...)
	return aca.Handle
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

func (this *AccessControlAllow) Handle(c *cosweb.Context) bool {
	header := c.Header()

	if len(this.origin) > 0 {
		header.Add("Access-Control-Allow-Origin", strings.Join(this.origin, ","))
	}
	if len(this.methods) > 0 {
		header.Add("Access-Control-Allow-Methods", strings.Join(this.methods, ","))
	}
	if len(this.headers) > 0 {
		header.Add("Access-Control-Allow-Headers", strings.Join(this.headers, ","))
		header.Add("Access-Control-Expose-Headers", strings.Join(this.headers, ","))
	}
	if this.Credentials {
		header.Set("Access-Control-Allow-Credentials", "true")
	}
	if this.expire != "" {
		header.Set("Access-Control-Max-Age", this.expire)
	}
	//UNITY - 仅在 Unity 请求时添加这些安全头
	userAgent := c.Request.Header.Get("User-Agent")
	if strings.Contains(strings.ToLower(userAgent), "unity") {
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("X-Frame-Options", "DENY")
		header.Set("X-XSS-Protection", "1; mode=block")
	}
	if c.Request.Method == http.MethodOptions {
		_, _ = c.Response.Write([]byte("options OK"))
		return false
	}
	return true
}
