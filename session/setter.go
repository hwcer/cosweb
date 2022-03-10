package session

import (
	"github.com/hwcer/cosgo/storage/cache"
	"sync/atomic"
	"time"
)

func NewSetter(id uint64, data interface{}) cache.Dataset {
	d := &Setter{
		locked: 1,
		Data:   *cache.NewData(),
	}
	d.Data.Reset(id, data)
	if Options.MaxAge > 0 {
		d.Expire(Options.MaxAge)
	}
	return d
}

type Setter struct {
	uid    string
	expire int64 //过期时间
	locked int32 //SESSION锁
	cache.Data
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
