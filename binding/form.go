// Copyright 2014 Manu Martinez-Almeida.  All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package binding

import (
	"fmt"
	"github.com/hwcer/cosgo/values"
	"github.com/hwcer/cosmo/schema"
	"io"
	"reflect"
)

var formBindingSchema = schema.New()

func init() {
	j := formBinding{}
	Register(MIMEPOSTForm, j)
}

type formBinding struct{}

func (formBinding) Name() string {
	return "form"
}

type bodyParseForm interface {
	ParseForm() (values.Values, error)
}

func (formBinding) Bind(body io.Reader, obj interface{}) (err error) {
	if body == nil {
		return fmt.Errorf("invalid request")
	}
	data := values.Values{}
	if i, ok := body.(bodyParseForm); ok {
		data, err = i.ParseForm()
	}
	if err != nil {
		return nil
	}

	vf := reflect.ValueOf(obj)
	s, err := formBindingSchema.Parse(vf)
	if err != nil {
		return nil
	}
	for _, field := range s.Fields {
		switch field.IndirectFieldType.Kind() {
		case reflect.String:
			field.Set(vf, data.GetString(field.DBName))
		case reflect.Int:
			field.Set(vf, data.GetInt(field.DBName))
		case reflect.Int32:
			field.Set(vf, data.GetInt32(field.DBName))
		case reflect.Int64:
			field.Set(vf, data.GetInt64(field.DBName))
		case reflect.Float32:
			field.Set(vf, data.GetFloat32(field.DBName))
		case reflect.Float64:
			field.Set(vf, data.GetFloat64(field.DBName))
		}
	}
	return nil
}
