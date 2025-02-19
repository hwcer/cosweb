package cosweb

import (
	"github.com/hwcer/cosgo"
	"github.com/hwcer/cosgo/logger"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const iStaticRoutePath = "_StaticRoutePath"

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
	arr := []string{strings.TrimSuffix(this.prefix, "/"), "*" + iStaticRoutePath}
	r = append(r, strings.Join(arr, "/"))
	return
}

func (this *Static) handle(c *Context, next Next) error {
	name := c.GetString(iStaticRoutePath, RequestDataTypeParam)
	var file string
	if !strings.Contains(name, ".") {
		file = filepath.Join(this.root, name, this.index)
		if _, err := os.Stat(file); err != nil {
			return next()
		}
	} else {
		file = filepath.Join(this.root, name)
	}

	http.ServeFile(c.Response, c.Request, file)
	return nil
}
