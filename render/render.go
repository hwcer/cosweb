package render

import (
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hwcer/logger"
)

// Render provides functions for easily writing HTML templates & JSON out to a HTTP Response.
type Render struct {
	Options   *Options
	templates map[string]*template.Template
}

// Options holds the configuration Options for a Render
type Options struct {
	Ext string
	// With Debug set to true, templates will be recompiled before every render call.
	Debug bool
	// The glob string to your templates
	Includes  string
	Templates string

	// The Glob string for additional templates
	//PartialsGlob string

	// The function map to pass to each HTML template
	Funcs template.FuncMap

	// Charset for responses
	Charset string

	Delims []string
}

// New creates a new Render with the given Options
func New(opts *Options) *Render {
	if opts == nil {
		opts = &Options{}
	}
	if opts.Ext == "" {
		opts.Ext = ".html"
	} else if !strings.HasPrefix(opts.Ext, ".") {
		opts.Ext = "." + opts.Ext
	}
	opts.Ext = strings.ToLower(opts.Ext)

	if opts.Funcs == nil {
		opts.Funcs = make(template.FuncMap)
	}
	if opts.Charset == "" {
		opts.Charset = "UTF-8"
	}

	r := &Render{
		Options:   opts,
		templates: make(map[string]*template.Template),
	}
	// 启动时编译一次,失败通过 logger.Alert 报告,不杀进程
	if err := r.compileTemplatesFromDir(); err != nil {
		logger.Alert("render compile templates: %v", err)
	}
	return r
}

// HTML executes the template and writes to the responsewriter
func (r *Render) Render(buf io.Writer, name string, data any) error {
	// re-compile on every render call when Debug is true
	if !strings.HasSuffix(name, r.Options.Ext) {
		name += r.Options.Ext
	}
	file := filepath.Join(r.Options.Templates, name)
	tplName, err := r.tplName(file)
	if err != nil {
		return err
	}
	if r.Options.Debug {
		if e := r.compileTemplatesFromDir(); e != nil {
			return e
		}
	}
	tmpl := r.templates[tplName]
	if tmpl == nil {
		return fmt.Errorf("unrecognised template %s", tplName)
	}
	return tmpl.Execute(buf, data)
}

func (r *Render) compileTemplatesFromDir() error {
	if r.Options.Templates == "" {
		return nil
	}
	var includes []string
	if r.Options.Includes != "" {
		inc, err := r.Glob(r.Options.Includes)
		if err != nil {
			return fmt.Errorf("glob includes: %w", err)
		}
		includes = inc
	}
	bases, err := r.Glob(r.Options.Templates)
	if err != nil {
		return fmt.Errorf("glob templates: %w", err)
	}

	baseTmpl := template.New("").Funcs(r.Options.Funcs)
	if len(r.Options.Delims) >= 2 {
		baseTmpl.Delims(r.Options.Delims[0], r.Options.Delims[1])
	}
	if len(includes) > 0 {
		if baseTmpl, err = baseTmpl.ParseFiles(includes...); err != nil {
			return fmt.Errorf("parse includes: %w", err)
		}
	}

	templates := make(map[string]*template.Template, len(bases))
	for _, templateFile := range bases {
		fileName, _ := r.tplName(templateFile)
		tmpl, err := baseTmpl.Clone()
		if err != nil {
			return fmt.Errorf("clone base: %w", err)
		}
		tmpl = tmpl.New(filepath.Base(templateFile))
		if tmpl, err = tmpl.ParseFiles(templateFile); err != nil {
			return fmt.Errorf("parse %s: %w", templateFile, err)
		}
		templates[fileName] = tmpl
	}
	r.templates = templates
	return nil
}

func (r *Render) tplName(file string) (string, error) {
	var err error
	file, err = filepath.Rel(r.Options.Templates, file)
	if err != nil {
		return "", err
	}
	//file = strings.ReplaceAll(file, "\\", "-")
	//file = strings.ReplaceAll(file, "/", "-")
	return file, nil
}

func (r *Render) Glob(root string) ([]string, error) {
	var tpl []string
	bases, err := filepath.Glob(root + "/*")
	if err != nil {
		return nil, err
	}
	for _, f := range bases {
		if s, e1 := os.Stat(f); e1 != nil {
			return nil, e1
		} else if s.IsDir() {
			if fs, e2 := r.Glob(f); e2 != nil {
				return nil, e2
			} else {
				tpl = append(tpl, fs...)
			}
		} else if strings.ToLower(filepath.Ext(f)) == r.Options.Ext {
			tpl = append(tpl, f)
		}
	}

	return tpl, nil
}
