package cosweb

import (
	"encoding/json"
	"encoding/xml"
	"strings"
)

type (
	// Binder is the interface that wraps the Bind value.
	Binder interface {
		Bind(c *Context, i interface{}) error
	}
	// DefaultBinder is the default implementation of the Binder interface.
	DefaultBinder struct{}
)

// Bind implements the `Packer#Bind` function.
func (b *DefaultBinder) Bind(c *Context, i interface{}) (err error) {
	if c.Body.Len() == 0 {
		return
	}
	ctype := strings.ToLower(c.Request.Header.Get(HeaderContentType))
	switch {
	case strings.HasPrefix(ctype, ContentTypeApplicationJSON):
		return json.NewDecoder(c.Body).Decode(i)
	case strings.HasPrefix(ctype, ContentTypeApplicationXML), strings.HasPrefix(ctype, ContentTypeTextXML):
		return xml.NewDecoder(c.Body).Decode(i)
	default:
		return ErrUnsupportedMediaType
	}
}
