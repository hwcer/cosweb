package cosweb

import (
	"fmt"
	"net/http"

	"github.com/hwcer/logger"
)

// HTTPErrorHandler 仅仅处理系统错误,必定返回非200错误码
var HTTPErrorHandler = func(c *Context, format any, args ...any) {
	defer func() {
		if err := recover(); err != nil {
			logger.Error(err)
		}
	}()
	if !c.Response.CanWrite() {
		return
	}
	he := NewHTTPError(0, format, args...)
	if he.Code == 0 || he.Code == http.StatusOK {
		he.Code = http.StatusInternalServerError
	}
	c.WriteHeader(he.Code)
	if he.Message == "" {
		he.Message = http.StatusText(he.Code)
	}
	if _, err := c.Response.Write([]byte(he.Message)); err != nil {
		logger.Error(err)
	}
}

// HTTPError represents an error that occurred while handling a Request.
type HTTPError struct {
	Code    int    `json:"-"`
	Message string `json:"message"`
}

// Errors
var (
	ErrNotFound             = NewHTTPError(http.StatusNotFound, http.StatusText(http.StatusNotFound))
	ErrForbidden            = NewHTTPError(http.StatusForbidden, http.StatusText(http.StatusForbidden))
	ErrInternalServerError  = NewHTTPError(http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
	ErrInvalidCertOrKeyType = NewHTTPError(0, "invalid cert or key type, must be string or []byte")
	ErrHandlerError         = NewHTTPError(0, "handler type error")

	ErrValidatorNotRegistered = NewHTTPError(0, "validator not registered")
	ErrRendererNotRegistered  = NewHTTPError(0, "renderer not registered")
	ErrInvalidRedirectCode    = NewHTTPError(0, "invalid redirect status code")
	ErrCookieNotFound         = NewHTTPError(0, "cookie not found")
	ErrArgsNotFound           = NewHTTPError(0, "args not found")
	ErrMimeTypeNotFound       = NewHTTPError(0, "mime type not found")
)

// Error makes it compatible with `error` interface.
func (he *HTTPError) Error() string {
	return he.String()
}

func (he *HTTPError) String() string {
	if he.Message != "" {
		return he.Message
	} else {
		code := he.Code
		if code == 0 {
			code = http.StatusOK
		}
		return http.StatusText(code)
	}
}

// NewHTTPError creates a new HTTPError instance.
func NewHTTPError(code int, format any, args ...any) *HTTPError {
	switch r := format.(type) {
	case HTTPError:
		return &r
	case *HTTPError:
		return r
	}
	he := &HTTPError{Code: code}
	if format == nil {
		return he
	}
	switch r := format.(type) {
	case error:
		he.Message = r.Error()
	case string:
		he.Message = fmt.Sprintf(r, args...)
	case []byte:
		he.Message = fmt.Sprintf(string(r), args...)
	default:
		he.Message = fmt.Sprintf(fmt.Sprintf("%v", r), args...)
	}
	return he
}

func NewHTTPError500(message any) *HTTPError {
	return NewHTTPError(http.StatusInternalServerError, message)
}
