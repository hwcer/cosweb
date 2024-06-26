package cosweb

import (
	"bytes"
	"github.com/hwcer/cosgo/binder"
	"github.com/hwcer/cosgo/values"
	"io"
	"net/http"
)

func NewBody() *Body {
	return &Body{}
}

type Body struct {
	bytes  []byte
	binder binder.Interface
	values values.Values
	//errors error
}

func (this *Body) reset(req *http.Request) {
	ct := req.Header.Get(HeaderContentType)
	if ct == "" {
		ct = ContentTypeApplicationJSON
	}
	this.binder = binder.Handle(ct)
	if this.binder == nil {
		return
	}
	if err := this.readAll(req.Body); err != nil {
		this.bytes = nil
		return
	}
	req.Body = io.NopCloser(bytes.NewReader(this.bytes))
}
func (this *Body) release() {
	this.binder = nil
	this.values = nil
}

func (this *Body) Get(key string) (val interface{}, ok bool) {
	params, err := this.Values()
	if err == nil {
		val, ok = params[key]
	}
	return
}

func (this *Body) Bind(i interface{}) error {
	if this.binder == nil {
		return ErrMimeTypeNotFound
	}
	v := this.Bytes()
	if len(v) == 0 {
		return nil
	}
	return this.binder.Unmarshal(v, i)
}

func (this *Body) Bytes() []byte {
	return this.bytes
}

func (this *Body) Reader() (r io.Reader, err error) {
	r = bytes.NewReader(this.Bytes())
	return
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

// TODO 优化
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
