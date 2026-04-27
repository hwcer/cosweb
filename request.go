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

// 默认获取数据的顺序。Context 放在最前,让用户 c.Set(k, v) 的值优先于外部输入,
// 避免与同名 URL 参数混淆。
var defaultRequestDataType = RequestDataTypeMap{RequestDataTypeContext, RequestDataTypeParam, RequestDataTypeQuery, RequestDataTypeBody, RequestDataTypeCookie}

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
