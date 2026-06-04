package cosweb

import (
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/hwcer/cosgo"
	"github.com/hwcer/logger"
)

func init() {
	_ = mime.AddExtensionType(".mjs", "text/javascript")
}

type Static struct {
	root    string
	index   string
	nocache bool
}

func NewStatic(root string) *Static {
	return &Static{
		root:  cosgo.Abs(root),
		index: "index.html",
	}
}

func (this *Static) Index(f string) {
	if !strings.Contains(f, ".") {
		logger.Alert("static index file error:%v", f)
	} else {
		this.index = f
	}
}

func (this *Static) Nocache(v bool) {
	this.nocache = v
}

func (this *Static) Handle(c *Context) any {
	name := c.GetString("*", RequestDataTypeParam)
	if name == "" {
		name = this.index
	}
	safe := filepath.Clean("/" + name)
	var file string
	if !strings.Contains(safe, ".") {
		file = filepath.Join(this.root, safe, this.index)
	} else {
		file = filepath.Join(this.root, safe)
	}
	if !withinRoot(this.root, file) || !fileExists(file) {
		return ErrNotFound
	}
	this.serveFile(c, file)
	return nil
}

func (this *Static) serveFile(c *Context, file string) {
	if this.nocache {
		h := c.Response.Header()
		h.Set("Cache-Control", "no-cache, no-store, must-revalidate")
		h.Set("Pragma", "no-cache")
		h.Set("Expires", "0")
		c.Request.Header.Del("If-Modified-Since")
		c.Request.Header.Del("If-None-Match")
	}
	http.ServeFile(c.Response, c.Request, file)
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

func withinRoot(root, file string) bool {
	rel, err := filepath.Rel(root, file)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
