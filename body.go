package cosweb

import (
	"bytes"
	"github.com/hwcer/cosgo/values"
	"github.com/hwcer/cosweb/binding"
	"io"
	"net/http"
)

func NewBody() *Body {
	return &Body{}
}

type Body struct {
	err    error
	req    *http.Request
	bytes  []byte
	params values.Values
}

func (b *Body) Reset(req *http.Request) {
	b.req = req
}
func (b *Body) Release() {
	b.err = nil
	b.req = nil
	b.bytes = nil
	b.params = nil
}

//func (b *Body) Len() (r int) {
//	v, err := b.Bytes()
//	if err == nil {
//		r = len(v)
//	}
//	return
//}

func (b *Body) Get(key string) (val interface{}, ok bool) {
	params, err := b.Values()
	if err == nil {
		val, ok = params[key]
	}
	return
}

// Read 非多线程安全
//func (b *Body) Read(p []byte) (int, error) {
//	v, err := b.Bytes()
//	if err != nil {
//		return 0, err
//	}
//	if len(v) <= b.off {
//		b.off = 0
//		return 0, io.EOF
//	}
//	n := copy(p, v[b.off:])
//	b.off += n
//	return n, nil
//}

func (b *Body) Bind(i interface{}) error {
	ct := b.req.Header.Get(HeaderContentType)
	h := binding.Handle(ct)
	if h == nil {
		return ErrMimeTypeNotFound
	}
	v, err := b.Bytes()
	if err != nil {
		return err
	}
	if len(v) == 0 {
		return nil
	}
	return h.Unmarshal(v, i)
}

func (b *Body) Bytes() ([]byte, error) {
	if b.err != nil {
		return nil, b.err
	}
	if b.bytes == nil {
		if b.bytes, b.err = io.ReadAll(b.req.Body); b.err == nil {
			b.req.Body = io.NopCloser(bytes.NewReader(b.bytes))
		} else if b.bytes == nil {
			b.bytes = make([]byte, 0, 1)
		}
	}
	return b.bytes, nil
}

func (b *Body) Reader() io.Reader {
	d, _ := b.Bytes()
	return bytes.NewReader(d)
}

func (b *Body) Values() (values.Values, error) {
	if b.params == nil {
		b.params = make(values.Values, 0)
		if err := b.Bind(&b.params); err != nil {
			return nil, err
		}
	}
	return b.params, nil
}
