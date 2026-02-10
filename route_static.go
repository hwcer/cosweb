package cosweb

import (
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/hwcer/cosgo"
	"github.com/hwcer/cosgo/registry"
	"github.com/hwcer/logger"
)

//const StaticRoutePath = "_StaticRoutePath"

func init() {
	_ = mime.AddExtensionType(".mjs", "text/javascript")
}

//func (c *Context) FileServer() bool {
//	//s := c.GetString(StaticRoutePath, RequestDataTypeParam)
//	//if s == "" {
//	//	return false
//	//}
//	p := path.Clean(c.Request.URL.Path)
//	return strings.HasSuffix(p, s)
//}

type Static struct {
	root       string
	index      string
	prefix     string
	middleware []MiddlewareFunc
}

func NewStatic(prefix string, root string) *Static {
	prefix = registry.Route(prefix)
	s := &Static{prefix: prefix, root: cosgo.Abs(root)}
	s.index = "index.html"
	return s
}

func (this *Static) Use(middleware ...MiddlewareFunc) {
	this.middleware = append(this.middleware, middleware...)
}

func (this *Static) Index(f string) {
	if !strings.Contains(f, ".") {
		logger.Alert("static index file error:%v", f)
	} else {
		this.index = f
	}
}
func (this *Static) Route() (r string) {
	prefix := strings.TrimRight(this.prefix, "/")
	if prefix == "" {
		return "*"
	}
	return fmt.Sprintf("%s/*", this.prefix)
}

func (this *Static) handle(c *Context) any {
	middleware := append([]MiddlewareFunc{}, this.middleware...)
	middleware = append(middleware, func(context *Context, next Next) error {
		name := c.GetString(registry.PathMatchVague, RequestDataTypeParam)
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
	})

	return c.doMiddleware(middleware)

}
