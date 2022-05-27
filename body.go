package cosweb

import (
	"bytes"
	"github.com/hwcer/cosweb/binding"
	"io"
)

func NewBody(c *Context) *Body {
	return &Body{c: c}
}

type Body struct {
	c      *Context
	Error  error
	buffer *bytes.Buffer
	params map[string]interface{}
}

func (b *Body) release() {
	b.Error = nil
	b.params = nil
	b.buffer = nil
}

func (b *Body) Len() (r int) {
	buf := b.Buffer()
	if b.Error == nil {
		r = buf.Len()
	}
	return
}

func (b *Body) Get(key string) (val interface{}, ok bool) {
	if b.params == nil {
		b.params = make(map[string]interface{}, 0)
		_ = b.Bind(&b.params)
	}
	val, ok = b.params[key]
	return
}

func (b *Body) Read(p []byte) (n int, err error) {
	buf := b.Buffer()
	if b.Error != nil {
		return 0, b.Error
	}
	n = copy(p, buf.Bytes())
	return
}

func (b *Body) Bind(i interface{}) error {
	ct := b.c.Request.Header.Get(HeaderContentType)
	h := binding.Handle(ct)
	if h == nil {
		return ErrMimeTypeNotFound
	}
	if b.Len() == 0 {
		return nil
	}
	return h.Bind(b, i)
}
func (b *Body) Bytes() (r []byte) {
	buf := b.Buffer()
	if b.Error == nil {
		r = buf.Bytes()
	}
	return
}

func (b *Body) Buffer() *bytes.Buffer {
	if b.buffer == nil {
		b.buffer = &bytes.Buffer{}
		reader := io.LimitReader(b.c.Request.Body, defaultMemory)
		_, b.Error = b.buffer.ReadFrom(reader)
	}
	return b.buffer
}
