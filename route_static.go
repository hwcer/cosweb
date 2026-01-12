package cosweb

import (
	"fmt"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/hwcer/cosgo"
	"github.com/hwcer/cosgo/registry"
	"github.com/hwcer/logger"
)

const StaticRoutePath = "_StaticRoutePath"

func init() {
	_ = mime.AddExtensionType(".mjs", "text/javascript")
}

func (c *Context) FileServer() bool {
	s := c.GetString(StaticRoutePath, RequestDataTypeParam)
	if s == "" {
		return false
	}
	p := path.Clean(c.Request.URL.Path)
	return strings.HasSuffix(p, s)
}

type Static struct {
	root   string
	index  string
	prefix string
}

func NewStatic(prefix string, root string) *Static {
	s := &Static{prefix: prefix, root: cosgo.Abs(root)}
	s.index = "index.html"
	return s
}
func (this *Static) Index(f string) {
	if !strings.Contains(f, ".") {
		logger.Alert("static index file error:%v", f)
	} else {
		this.index = f
	}
}
func (this *Static) Route() (r []string) {
	prefix := registry.Route(this.prefix)
	prefix = strings.TrimSuffix(prefix, "*")
	prefix = strings.TrimSuffix(prefix, "/")
	r = append(r, fmt.Sprintf("%s/*%s", prefix, StaticRoutePath))
	return
}

func (this *Static) handle(c *Context) any {
	name := c.GetString(StaticRoutePath, RequestDataTypeParam)
	if name == "" {
		name = this.index
	}
	var file string
	if !strings.Contains(name, ".") {
		file = filepath.Join(this.root, name, this.index)
		if _, err := os.Stat(file); err != nil {
			return c.Error(ErrNotFound)
		}
	} else {
		file = filepath.Join(this.root, name)
	}
	c.Response.hijacked = true
	http.ServeFile(c.Response.ResponseWriter, c.Request, file)

	return nil
}
