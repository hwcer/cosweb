package cosweb

import "github.com/hwcer/cosgo/binder"

var Charset = "UTF-8"

type ContentType string

// Headers
const (
	HeaderAccept              = "Accept"
	HeaderAcceptEncoding      = "Accept-Encoding"
	HeaderAllow               = "Allow"
	HeaderAuthorization       = "Authorization"
	HeaderContentDisposition  = "Content-Disposition"
	HeaderContentEncoding     = "Content-Encoding"
	HeaderContentLength       = "Content-Length"
	HeaderContentType         = "Content-Type"
	HeaderCookie              = "Cookie"
	HeaderSetCookie           = "Set-Cookie"
	HeaderIfModifiedSince     = "If-Modified-Since"
	HeaderLastModified        = "Last-Modified"
	HeaderLocation            = "Location"
	HeaderUpgrade             = "Upgrade"
	HeaderVary                = "Vary"
	HeaderWWWAuthenticate     = "WWW-Authenticate"
	HeaderXForwardedFor       = "X-Forwarded-For"
	HeaderXForwardedProto     = "X-Forwarded-Proto"
	HeaderXForwardedProtocol  = "X-Forwarded-Protocol"
	HeaderXForwardedSsl       = "X-Forwarded-Ssl"
	HeaderXUrlScheme          = "X-Url-Scheme"
	HeaderXHTTPMethodOverride = "X-HTTP-Method-Override"
	HeaderXRealIP             = "X-Real-IP"
	HeaderXRequestID          = "X-Request-ID"
	HeaderXRequestedWith      = "X-Requested-With"
	HeaderServer              = "Server"
	HeaderOrigin              = "Origin"

	// Access control
	HeaderAccessControlRequestMethod    = "Access-Control-Request-Method"
	HeaderAccessControlRequestHeaders   = "Access-Control-Request-Headers"
	HeaderAccessControlAllowOrigin      = "Access-Control-Allow-Origin"
	HeaderAccessControlAllowMethods     = "Access-Control-Allow-Methods"
	HeaderAccessControlAllowHeaders     = "Access-Control-Allow-Headers"
	HeaderAccessControlAllowCredentials = "Access-Control-Allow-Credentials"
	HeaderAccessControlExposeHeaders    = "Access-Control-Expose-Headers"
	HeaderAccessControlMaxAge           = "Access-Control-Max-Age"

	// Security
	HeaderStrictTransportSecurity         = "Strict-Transport-Security"
	HeaderXContentTypeOptions             = "X-Content-Type-Options"
	HeaderXXSSProtection                  = "X-XSS-Protection"
	HeaderXFrameOptions                   = "X-Frame-Options"
	HeaderContentSecurityPolicy           = "Content-Security-Policy"
	HeaderContentSecurityPolicyReportOnly = "Content-Security-Policy-Report-Only"
	HeaderXCSRFToken                      = "X-CSRF-Token"
	HeaderReferrerPolicy                  = "Referrer-Policy"
)

// MIME types
const (
	ContentTypeTextHTML              ContentType = "text/html"
	ContentTypeTextPlain             ContentType = "text/plain"
	ContentTypeTextXML               ContentType = "text/xml"
	ContentTypeApplicationJS         ContentType = "application/javascript"
	ContentTypeApplicationXML        ContentType = "application/xml"
	ContentTypeApplicationJSON       ContentType = "application/json"
	ContentTypeApplicationProtobuf   ContentType = "application/protobuf"
	ContentTypeApplicationMsgpack    ContentType = "application/msgpack"
	ContentTypePROTOBUF              ContentType = "application/x-protobuf"
	ContentTypeMSGPACKX              ContentType = "application/x-msgpack"
	ContentTypeOctetStream           ContentType = "application/octet-stream"
	ContentTypeMultipartForm         ContentType = "multipart/form-data"
	ContentTypeApplicationForm       ContentType = "application/x-www-form-urlencoded"
	ContentTypeApplicationJavaScript ContentType = "application/javascript"
)

// GetContentTypeCharset
func GetContentTypeCharset(contentType ContentType) string {
	return string(contentType) + "; charset=" + Charset
}

func init() {
	//默认非静态文件和模版引擎的情况下，浏览器请求返回的数据使用JSON序列化
	ct := string(ContentTypeTextHTML)
	binder.SetMimeType(100, "HTML", ct)
	_ = binder.Register(ct, binder.Json)
}
