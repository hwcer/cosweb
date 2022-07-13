package render

import (
	"fmt"
	"github.com/hwcer/cosgo/logger"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"strings"
)

//var funcs template.FuncMap
//
//func init() {
//	funcs = make(template.FuncMap)
//	funcs["unescaped"] = unescaped
//}
//func unescaped(x string) interface{} {
//	return template.URL(x)
//}

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
	//for k, v := range funcs {
	//	if _, ok := opts.Funcs[k]; !ok {
	//		opts.Funcs[k] = v
	//	}
	//}

	if opts.Charset == "" {
		opts.Charset = "UTF-8"
	}

	r := &Render{
		Options:   opts,
		templates: make(map[string]*template.Template),
	}

	r.compileTemplatesFromDir()
	return r
}

// HTML executes the template and writes to the responsewriter
func (r *Render) Render(buf io.Writer, name string, data interface{}) error {
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
		r.compileTemplatesFromDir()
	}
	tmpl := r.templates[tplName]
	if tmpl == nil {
		return fmt.Errorf("unrecognised template %s", tplName)
	}
	// execute template
	err = tmpl.Execute(buf, data)
	if err != nil {
		return err
	}
	return nil
}

func (r *Render) compileTemplatesFromDir() {
	if r.Options.Templates == "" {
		return
	}
	templates := make(map[string]*template.Template)
	var err error
	var bases []string
	var includes []string
	if r.Options.Includes != "" {
		includes, err = r.Glob(r.Options.Includes)
		if err != nil {
			logger.Fatal(err.Error())
		}
	}
	bases, err = r.Glob(r.Options.Templates)
	if err != nil {
		logger.Fatal(err.Error())
	}

	baseTmpl := template.New("").Funcs(r.Options.Funcs)
	if len(r.Options.Delims) >= 2 {
		baseTmpl.Delims(r.Options.Delims[0], r.Options.Delims[1])
	}

	// parse partials (glob)
	if len(includes) > 0 {
		baseTmpl = template.Must(baseTmpl.ParseFiles(includes...))
	}

	for _, templateFile := range bases {
		//tplName := filepath.Base(templateFile)
		fileName, _ := r.tplName(templateFile)
		// set template name
		tmpl := template.Must(baseTmpl.Clone())
		tmpl = tmpl.New(filepath.Base(templateFile))
		// parse child template
		tmpl = template.Must(tmpl.ParseFiles(templateFile))
		templates[fileName] = tmpl
	}
	r.templates = templates
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
