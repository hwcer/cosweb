package cosweb

import (
	"errors"
	"github.com/hwcer/cosgo/logger"
	"net/http"
)

var Errorf HTTPErrorHandler = defaultHTTPErrorHandler

// DefaultHTTPErrorHandler is the default HTTP error handler. It sends a JSON Response
// with status code.
func defaultHTTPErrorHandler(c *Context, err error) {
	he := &HTTPError{}
	if !errors.As(err, &he) {
		he = NewHTTPError(http.StatusInternalServerError, err)
	}
	c.WriteHeader(he.Code)
	if c.Request.Method != http.MethodHead {
		_ = c.Bytes(ContentTypeTextPlain, []byte(he.String()))
	}
	if he.Code != http.StatusNotFound && he.Code != http.StatusInternalServerError {
		logger.Error(he)
	}
}
