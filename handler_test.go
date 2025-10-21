package cosweb

import (
	"testing"
)

func TestName(t *testing.T) {
	if !useFunc(login) {
		t.Log("login useFunc 失败")
	} else {
		t.Log("login useFunc 成功")
	}

	if !useHandle(login) {
		t.Log("login useHandle 失败")
	} else {
		t.Log("login useHandle 成功")
	}

	th := h{}

	if !useFunc(th.login) {
		t.Log("h.login useFunc 失败")
	} else {
		t.Log("h.login useFunc 成功")
	}

	if !useHandle(th.login) {
		t.Log("h.login useHandle 失败")
	} else {
		t.Log("h.login useHandle 成功")
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
