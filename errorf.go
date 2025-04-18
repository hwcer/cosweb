package cosweb

import (
	"github.com/hwcer/cosgo/values"
	"strings"
)

var Errorf HTTPErrorHandler = defaultHTTPErrorHandler

// DefaultHTTPErrorHandler is the default HTTP error handler. It sends a JSON Response
// with status code.
func defaultHTTPErrorHandler(c *Context, err error) {
	if ct := c.Request.Header.Get(HeaderAccept); ct != "" && strings.Contains(ct, string(ContentTypeTextHTML)) {
		_ = c.HTML(err.Error())
	} else {
		_ = c.JSON(values.Error(err))
	}
}
