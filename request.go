package cosweb

type RequestDataType int
type RequestDataTypeMap []RequestDataType

const (
	RequestDataTypeParam   RequestDataType = iota //params
	RequestDataTypeBody                           //POST json, xml,pb,form....
	RequestDataTypeQuery                          //GET
	RequestDataTypeCookie                         //COOKIES
	RequestDataTypeHeader                         //HEADER
	RequestDataTypeContext                        //context 上下文数据，必须先c.Set(k ,v)
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

func getDataFromRequest(c *Context, key string, dataType RequestDataType) (any, bool) {
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
	case RequestDataTypeContext:
		v, ok := c.context[key]
		return v, ok
	}
	return "", false
}
func getBodyValue(c *Context, k string) (v any, ok bool) {
	vs := c.Values()
	if ok = vs.Has(k); ok {
		v = vs.Get(k)
	}
	return
}

func getQueryValue(c *Context, k string) (v string, ok bool) {
	if c.query == nil {
		c.query = c.Request.URL.Query()
	}
	if ok = c.query.Has(k); ok {
		v = c.query.Get(k)
	}
	return
}
