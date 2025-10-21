package cosweb

import (
	"bufio"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/hwcer/cosgo/values"
)

type Response struct {
	http.ResponseWriter
	status   int
	canWrite bool
	hijacked bool
}

func (res *Response) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := res.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("response does not implement http.Hijacker")
	}

	conn, buf, err := hijacker.Hijack()
	if err == nil {
		res.hijacked = true // 标记为已劫持
	}
	return conn, buf, err
}
func (res *Response) CanWrite() bool {
	return res.canWrite && !res.hijacked
}
func (res *Response) Write(b []byte) (n int, err error) {
	if !res.CanWrite() {
		return 0, nil
	}
	res.canWrite = false
	if res.status == 0 {
		res.WriteHeader(http.StatusOK)
	}
	return res.ResponseWriter.Write(b)
}
func (res *Response) WriteHeader(code int) {
	res.status = code
	res.ResponseWriter.WriteHeader(code)
}

func (c *Context) Header() http.Header {
	return c.Response.Header()
}

// Write writes the store to the connection as part of an HTTP reply.
func (c *Context) Write(reply any) error {
	b := c.Accept()
	switch v := reply.(type) {
	case []byte:
		return c.Bytes(ContentType(b.String()), v)
	case *[]byte:
		return c.Bytes(ContentType(b.String()), *v)
	default:
		msg := values.Parse(reply)
		data, err := b.Marshal(msg)
		if err != nil {
			return err
		} else {
			return c.Bytes(ContentType(b.String()), data)
		}
	}
}
func (c *Context) WriteHeader(code int) {
	c.Response.WriteHeader(code)
}

func (c *Context) writeContentType(contentType ContentType) {
	header := c.Header()
	header.Set(HeaderContentType, GetContentTypeCharset(contentType))
}

func (c *Context) contentDisposition(file, name, dispositionType string) error {
	header := c.Header()
	header.Set(HeaderContentDisposition, fmt.Sprintf("%s; filename=%q", dispositionType, name))
	return c.File(file)
}

func (c *Context) Bytes(contentType ContentType, b []byte) (err error) {
	c.writeContentType(contentType)
	_, err = c.Response.Write(b)
	return
}
func (c *Context) Render(name string, data interface{}) (err error) {
	if c.Server.Render == nil {
		return ErrRendererNotRegistered
	}
	buf := new(bytes.Buffer)
	if err = c.Server.Render.Render(buf, name, data); err != nil {
		return
	}
	return c.Bytes(ContentTypeTextHTML, buf.Bytes())
}

func (c *Context) File(file string) (err error) {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	fi, _ := f.Stat()
	if fi.IsDir() {
		file = filepath.Join(file, indexPage)
		f, err = os.Open(file)
		if err != nil {
			return err
		}
		defer f.Close()
		if fi, err = f.Stat(); err != nil {
			return
		}
	}
	http.ServeContent(c.Response, c.Request, fi.Name(), fi.ModTime(), f)
	return
}

func (c *Context) Stream(contentType ContentType, r io.Reader) (err error) {
	c.writeContentType(contentType)
	_, err = io.Copy(c.Response, r)
	return
}

// Inline 最终走File
func (c *Context) Inline(file, name string) error {
	return c.contentDisposition(file, name, "inline")
}

// Attachment 最终走File
func (c *Context) Attachment(file, name string) error {
	return c.contentDisposition(file, name, "attachment")
}

func (c *Context) Redirect(url string) error {
	c.Response.Header().Set(HeaderLocation, url)
	c.WriteHeader(http.StatusFound)
	return nil
}

func (c *Context) XML(i interface{}, indent string) (err error) {
	data, err := xml.Marshal(i)
	if err != nil {
		return err
	}
	return c.Bytes(ContentTypeApplicationXML, data)
}

func (c *Context) HTML(html string) (err error) {
	return c.Bytes(ContentTypeTextHTML, []byte(html))
}

func (c *Context) String(s string) (err error) {
	return c.Bytes(ContentTypeTextPlain, []byte(s))
}

func (c *Context) JSON(i interface{}) error {
	data, err := json.Marshal(i)
	if err != nil {
		return err
	}
	return c.Bytes(ContentTypeApplicationJSON, data)
}
