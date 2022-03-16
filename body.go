package cosweb

import (
	"github.com/hwcer/cosgo/library/logger"
	"github.com/hwcer/cosweb/binding"
	"io"
)

func NewBody(c *Context) *Body {
	return &Body{c: c}
}

type Body struct {
	c      *Context
	bytes  []byte
	params map[string]interface{}
}

func (b *Body) release() {
	b.bytes = nil
	b.params = nil
}

func (b *Body) Len() int {
	if _, err := b.Bytes(); err != nil {
		return 0
	}
	return len(b.bytes)
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
	if _, err = b.Bytes(); err != nil {
		return
	}
	n = copy(p, b.bytes)
	return
}

func (b *Body) Bytes() ([]byte, error) {
	if b.bytes == nil {
		var err error
		reader := io.LimitReader(b.c.Request.Body, defaultMemory)
		if b.bytes, err = io.ReadAll(reader); err != nil {
			return nil, err
		}
	}
	return b.bytes, nil
}

func (b *Body) Bind(i interface{}) error {
	ct := b.c.Request.Header.Get(HeaderContentType)
	h := binding.Handle(ct)
	if h == nil {
		return ErrMimeTypeNotFound
	}
	return h.Bind(b, i)
}
