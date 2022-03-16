package cosweb

import (
	"bytes"
	"github.com/hwcer/cosgo/library/logger"
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

func (b *Body) Len() int {
	return b.buffer.Len()
}

func (b *Body) Get(key string) (val interface{}, ok bool) {
	if b.params == nil {
		params := make(map[string]interface{}, 0)
		if err := b.c.Bind(&params); err != nil {
			logger.Error("BODY BIND Err:%v", err)
		}
		b.params = params
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
		reader := io.LimitReader(b.c.Request.Body, defaultMemory)
		b.buffer = &bytes.Buffer{}
		_, b.Error = b.buffer.ReadFrom(reader)
	}
	return b.buffer
}
