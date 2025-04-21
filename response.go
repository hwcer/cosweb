package cosweb

import (
	"bufio"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
)

func (c *Context) Header() http.Header {
	return c.Response.Header()
}

// Write writes the store to the connection as part of an HTTP reply.
func (c *Context) Write(b []byte) (n int, err error) {
	//c.state.Push(netStateTypeWriteComplete)
	//c.WriteHeader(0)
	n, err = c.Response.Write(b)
	//c.contentSize += int64(n)
	return
}

// Writable 是否可写，如果已经写入头则返回FALSE
func (c *Context) Writable() bool {
	return c.Response.Header().Get(HeaderContentType) == ""
}

// Status sends an HTTP Response header with status code. If Status is
// not called explicitly, the first call to Write will trigger an implicit
// Status(http.StatusOK). Thus explicit calls to Status are mainly
// used to send error codes.
func (c *Context) WriteHeader(code int) {
	c.Response.WriteHeader(code)
}

// Flush implements the http.Flusher interface to allow an HTTP handler to flush
// buffered store to the client.
// See [http.Flusher](https://golang.org/pkg/net/http/#Flusher)
func (c *Context) Flush() {
	c.Response.(http.Flusher).Flush()
}

// Hijack implements the http.Hijacker interface to allow an HTTP handler to
// take over the connection.
// See [http.Hijacker](https://golang.org/pkg/net/http/#Hijacker)
func (c *Context) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return c.Response.(http.Hijacker).Hijack()
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
	_, err = c.Write(b)
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
	http.ServeContent(c, c.Request, fi.Name(), fi.ModTime(), f)
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
