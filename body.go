package cosweb

import (
	"bytes"
	"github.com/hwcer/cosgo/binding"
	"github.com/hwcer/cosgo/values"
	"io"
	"net/http"
)

func NewBody() *Body {
	return &Body{}
}

type Body struct {
	err     error
	bytes   []byte
	values  values.Values
	request *http.Request
}

func (this *Body) reset(req *http.Request) {
	this.request = req
	this.err = this.readAll(req.Body)
	if this.err == nil {
		req.Body = io.NopCloser(bytes.NewReader(this.bytes))
	}
}
func (this *Body) release() {
	this.err = nil
	this.values = nil
	this.request = nil
}

func (this *Body) Get(key string) (val interface{}, ok bool) {
	params, err := this.Values()
	if err == nil {
		val, ok = params[key]
	}
	return
}

func (this *Body) Bind(i interface{}) error {
	ct := this.request.Header.Get(HeaderContentType)
	h := binding.Handle(ct)
	if h == nil {
		return ErrMimeTypeNotFound
	}
	v, err := this.Bytes()
	if err != nil {
		return err
	}
	if len(v) == 0 {
		return nil
	}
	return h.Unmarshal(v, i)
}

func (this *Body) Bytes() ([]byte, error) {
	return this.bytes, this.err
}

func (this *Body) Reader() (io.Reader, error) {
	d, err := this.Bytes()
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(d), nil
}

func (this *Body) Values() (values.Values, error) {
	if this.values == nil {
		this.values = make(values.Values, 0)
		if err := this.Bind(&this.values); err != nil {
			return nil, err
		}
	}
	return this.values, nil
}

func (this *Body) readAll(r io.Reader) error {
	b := this.bytes
	if b == nil {
		b = make([]byte, 0, 512)
	}
	var c int
	for {
		if c == cap(b) {
			b = append(b, 0)[:c]
		}
		n, err := r.Read(b[c:cap(b)])
		c += n
		b = b[:c]
		if err != nil {
			this.bytes = b
			if err == io.EOF {
				err = nil
			}
			return err
		}
	}
}
