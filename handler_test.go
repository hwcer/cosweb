package cosweb

import (
	"testing"
)

// TestHandlerTypeAssertions 验证注册 handler 时底层签名断言成立。
// useHandle 断言具名类型 HandlerFunc —— Go 类型系统里,一个未命名的
// func(*Context) any 字面量不自动匹配 HandlerFunc,所以预期失败。
func TestHandlerTypeAssertions(t *testing.T) {
	if !useFunc(login) {
		t.Errorf("login 应该满足 func(*Context) any 底层签名")
	}
	if useHandle(login) {
		t.Errorf("login 不应匹配具名类型 HandlerFunc")
	}

	th := h{}
	if !useFunc(th.login) {
		t.Errorf("h.login 应该满足 func(*Context) any 底层签名")
	}
	if useHandle(th.login) {
		t.Errorf("h.login 不应匹配具名类型 HandlerFunc")
	}
}

func useFunc(i any) bool {
	_, ok := i.(func(*Context) any)
	return ok
}

func useHandle(i any) bool {
	_, ok := i.(HandlerFunc)
	return ok
}

func login(*Context) any {
	return nil
}

type h struct {
}

func (h) login(*Context) any {
	return nil
}
