package session

import (
	"github.com/hwcer/cosgo/values"
)

type StartType uint8

const (
	StartTypeNone StartType = 0 //不需要验证登录
	StartTypeAuth StartType = 1 //需要登录
	StartTypeLock StartType = 2 //需要登录，并且锁定,用户级别防并发
)

func New() *Session {
	return &Session{}
}

type Session struct {
	//key    string
	uuid   string //store key
	token  string //session id
	dirty  []string
	locked bool
	values values.Values
}

func (this *Session) Start(token string, level StartType) (err error) {
	if Options.storage == nil {
		return ErrorStorageNotSet
	}
	this.token = token
	if level == StartTypeNone {
		return nil
	}
	if this.token == "" {
		return ErrorSessionIdEmpty
	}

	var lock bool
	if level == StartTypeLock {
		lock = true
	}

	if this.uuid, this.values, err = Options.storage.Get(this.token, lock); err != nil {
		return err
	} else if len(this.values) == 0 {
		return ErrorSessionNotExist
	}
	if lock {
		this.locked = lock
	}
	return nil
}

func (this *Session) Get(key string) (v interface{}) {
	if this.values != nil {
		v = this.values.Get(key)
	}
	return
}
func (this *Session) GetInt32(key string) (v int32) {
	if this.values != nil {
		v = this.values.GetInt32(key)
	}
	return
}
func (this *Session) GetInt64(key string) (v int64) {
	if this.values != nil {
		v = this.values.GetInt64(key)
	}
	return
}
func (this *Session) GetString(key string) (v string) {
	if this.values != nil {
		v = this.values.GetString(key)
	}
	return
}

func (this *Session) Set(key string, val interface{}) bool {
	if this.values == nil {
		return false
	}
	this.dirty = append(this.dirty, key)
	this.values[key] = val
	return true
}

func (this *Session) All() values.Values {
	data := make(values.Values, len(this.values))
	for k, v := range this.values {
		data.Set(k, v)
	}
	return data
}

//Create 创建SESSION，uuid 用户唯一ID，可以检测是不是重复登录
func (this *Session) Create(uuid string, data values.Values) (token string, err error) {
	if Options.storage == nil {
		return "", ErrorStorageNotSet
	}

	this.token, err = Options.storage.Create(uuid, data, Options.MaxAge, true)
	if err != nil {
		return "", err
	}
	this.uuid = uuid
	this.values = data
	this.locked = true
	return this.token, nil
}

func (this *Session) Delete() (err error) {
	if Options.storage == nil || this.uuid == "" {
		return nil
	}
	if err = Options.storage.Delete(this.uuid); err != nil {
		return
	}
	this.release()
	return
}

//Release 释放 session 由HTTP SERVER
func (this *Session) Release() {
	if this.uuid == "" || this.token == "" {
		return
	}
	data := make(values.Values)
	for _, k := range this.dirty {
		data[k] = this.values[k]
	}
	_ = Options.storage.Save(this.token, data, Options.MaxAge, this.locked)
	this.release()
}

func (this *Session) release() {
	this.uuid = ""
	this.token = ""
	this.dirty = nil
	this.locked = false
	this.values = nil
}
