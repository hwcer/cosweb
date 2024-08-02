package cosweb

import (
	"github.com/hwcer/cosgo/values"
	"github.com/hwcer/logger"
	"net/url"
)

type RequestDataType int
type RequestDataTypeMap []RequestDataType

const (
	RequestDataTypeParam  RequestDataType = iota //params
	RequestDataTypeBody                          //POST json, xml,pb,form....
	RequestDataTypeQuery                         //GET
	RequestDataTypeCookie                        //COOKIES
	RequestDataTypeHeader                        //HEADER
)

// 默认session id获取方式
//var defaultSessionDataType = RequestDataTypeMap{RequestDataTypeQuery, RequestDataTypeCookie, RequestDataTypeHeader}

// 默认获取数据的顺序
var defaultRequestDataType = RequestDataTypeMap{RequestDataTypeParam, RequestDataTypeQuery, RequestDataTypeBody, RequestDataTypeCookie}

func (r *RequestDataTypeMap) IndexOf(v RequestDataType) int {
	for i, t := range *r {
		if t == v {
			return i
		}
	}
	return -1
}

func (r *RequestDataTypeMap) Add(keys ...RequestDataType) {
	for _, k := range keys {
		if r.IndexOf(k) < 0 {
			*r = append(*r, k)
		}
	}
}
func (r *RequestDataTypeMap) Reset(keys ...RequestDataType) {
	*r = keys
}

func getDataFromRequest(c *Context, key string, dataType RequestDataType) (interface{}, bool) {
	switch dataType {
	case RequestDataTypeParam:
		v, ok := c.params[key]
		return v, ok
	case RequestDataTypeQuery:
		return getQueryValue(c, key)
	case RequestDataTypeBody:
		return getBodyValue(c, key)
	case RequestDataTypeCookie:
		if val, err := c.Request.Cookie(key); err == nil && val.Value != "" {
			return val.Value, true
		}
	case RequestDataTypeHeader:
		if v := c.Request.Header.Get(key); v != "" {
			return v, true
		}
	}
	return "", false
}
func getBodyValue(c *Context, k string) (v any, ok bool) {
	ct := c.ContentType()
	//FORM
	if ct == ContentTypeApplicationForm {
		_ = c.Request.ParseForm()
		if ok = c.Request.Form.Has(k); ok {
			v = c.Request.Form.Get(k)
		}
		return
	}
	//JSON
	if c.values == nil {
		c.values = values.Values{}
		if err := c.Bind(&c.values); err != nil {
			logger.Debug("url.ParseQuery Err:%v", err)
		}
	}
	if ok = c.values.Has(k); ok {
		v = c.values.Get(k)
	}
	return
}

func getQueryValue(c *Context, key string) (v string, ok bool) {
	if c.query == nil {
		var err error
		if c.query, err = url.ParseQuery(c.Request.URL.RawQuery); err != nil {
			logger.Debug("url.ParseQuery Err:%v", err)
			c.query = make(url.Values)
		}
	}
	if ok = c.query.Has(key); ok {
		v = c.query.Get(key)
	}
	return
}
