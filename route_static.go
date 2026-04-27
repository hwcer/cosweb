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

// Static 静态文件服务
// 注册为全局中间件（非路由），文件存在直接响应，不存在调 next() 回退到 API 路由匹配
type Static struct {
	root    string
	index   string
	prefix  string
	nocache bool
	methods map[string]bool
}

func NewStatic(prefix string, root string) *Static {
	if prefix != "/" {
		prefix = strings.TrimRight(prefix, "/")
	}
	return &Static{
		root:    cosgo.Abs(root),
		index:   "index.html",
		prefix:  prefix,
		methods: map[string]bool{http.MethodGet: true, http.MethodHead: true},
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

// Middleware 返回全局中间件函数
// 文件存在 → 响应并终止链（不调 next）
// 文件不存在或路径不匹配 → 调 next() 继续 API 路由

func (this *Static) Middleware(c *Context, next Next) error {
	if !this.methods[c.Request.Method] {
		return next()
	}
	path := c.Request.URL.Path
	if this.prefix != "/" && !strings.HasPrefix(path, this.prefix+"/") && path != this.prefix {
		return next()
	}

	name := strings.TrimPrefix(path, this.prefix)
	name = strings.TrimPrefix(name, "/")
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
		return next()
	}

	return this.serveFile(c, file)
}

func (this *Static) serveFile(c *Context, file string) error {
	if this.nocache {
		h := c.Response.Header()
		h.Set("Cache-Control", "no-cache, no-store, must-revalidate")
		h.Set("Pragma", "no-cache")
		h.Set("Expires", "0")
	}
	http.ServeFile(c.Response, c.Request, file)
	return nil
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

// withinRoot 检查 file 是否位于 root 目录内
func withinRoot(root, file string) bool {
	rel, err := filepath.Rel(root, file)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
