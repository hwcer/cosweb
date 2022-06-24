package session

import (
	"github.com/hwcer/cosgo/storage/cache"
	"github.com/hwcer/cosgo/values"
	"sync/atomic"
	"time"
)

func NewSetter(id uint64, data interface{}) cache.Interface {
	d := &Setter{
		locked: 1,
		Setter: cache.NewSetter(id, data),
	}
	if Options.MaxAge > 0 {
		d.Expire(Options.MaxAge)
	}
	return d
}

type Setter struct {
	expire int64 //过期时间
	locked int32 //SESSION锁
	*cache.Setter
}

func (this *Setter) Values() values.Values {
	v := this.Setter.Get()
	if v == nil {
		return nil
	}
	r, _ := v.(values.Values)
	return r
}

func (this *Setter) Lock() bool {
	return atomic.CompareAndSwapInt32(&this.locked, 0, 1)
}
func (this *Setter) UnLock() bool {
	return atomic.CompareAndSwapInt32(&this.locked, 1, 0)
}

//Expire 设置有效期(s)
func (this *Setter) Expire(s int64) {
	this.expire = time.Now().Unix() + s
}
