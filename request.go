package cosweb

// RequestDataType 请求数据类型
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

// getDataFromRequest 从请求中获取数据（保持兼容性）
func getDataFromRequest(c *Context, key string, dataType RequestDataType) (any, bool) {
	return c.getDataFromStore(key, dataType)
}

// getQueryValue 从查询参数中获取值（保持兼容性）
func getQueryValue(c *Context, key string) (string, bool) {
	v, ok := c.getDataFromStore(key, RequestDataTypeQuery)
	if !ok {
		return "", false
	}
	if s, ok := v.(string); ok {
		return s, true
	}
	return "", false
}

// getBodyValue 从请求体中获取值（保持兼容性）
func getBodyValue(c *Context, key string) (any, bool) {
	return c.getDataFromStore(key, RequestDataTypeBody)
}
