package session

import (
	"github.com/hwcer/cosgo/storage/cache"
	"github.com/hwcer/cosgo/values"
	"time"
)

var Heartbeat int32 = 10 //心跳(S)

func NewMemory() *Memory {
	s := &Memory{
		Cache: cache.New(1024),
	}
	s.Cache.NewSetter = NewSetter
	return s
}

type Memory struct {
	*cache.Cache
	stop chan struct{}
}

func (this *Memory) Start() error {
	if Options.MaxAge > 0 {
		this.stop = make(chan struct{})
		go this.worker()
	}
	return nil
}
func (this *Memory) get(key string) (*Setter, error) {
	var (
		ok  bool
		id  uint64
		err error
	)
	if id, err = cache.Decode(key); err != nil {
		return nil, err
	}
	var data cache.Interface
	if data, ok = this.Cache.Get(id); !ok || data == nil {
		return nil, ErrorSessionNotExist
	}
	var val *Setter
	if val, ok = data.(*Setter); !ok {
		return nil, ErrorSessionTypeError
	}
	if val.expire > 0 && val.expire < time.Now().Unix() {
		return nil, ErrorSessionTypeExpire
	}
	return val, nil
}

func (this *Memory) Get(token string, lock bool) (uuid string, result values.Values, err error) {
	var ok bool
	var data *Setter
	if uuid, err = Decode(token); err != nil {
		return
	}
	if data, err = this.get(uuid); err != nil {
		return
	}
	if lock && !data.Lock() {
		return "", nil, ErrorSessionLocked
	}

	var val values.Values
	if val, ok = data.Get().(values.Values); !ok {
		return "", nil, ErrorSessionTypeError
	}
	result = make(values.Values, len(val))
	for k, v := range val {
		result.Set(k, v)
	}
	return
}

func (this *Memory) Save(uuid string, data values.Values, ttl int64, unlock bool) (err error) {
	var setter *Setter
	if setter, err = this.get(uuid); err != nil {
		return err
	}
	value := setter.Values()
	if value == nil {
		return ErrorSessionTypeError
	}

	for k, v := range value {
		if !data.Has(k) {
			data.Set(k, v)
		}
	}

	setter.Set(data)
	if ttl > 0 {
		setter.Expire(ttl)
	}
	if unlock {
		setter.UnLock()
	}
	return
}

//Create 创建新SESSION,返回SESSION Index
//Create会自动设置有效期
//Create新数据为锁定状态
func (this *Memory) Create(uuid string, data values.Values, ttl int64, lock bool) (token string, err error) {
	i := this.Cache.Push(data)
	token, err = Encode(cache.Encode(i.Id()))
	setter, _ := i.(*Setter)
	if ttl > 0 {
		setter.Expire(ttl)
	}
	if lock {
		setter.Lock()
	}
	return
}

func (this *Memory) Delete(key string) error {
	id, err := cache.Decode(key)
	if err != nil {
		return err
	}
	this.Cache.Delete(id)
	return nil
}

func (this *Memory) Close() error {
	if Options.MaxAge == 0 || this.stop == nil {
		return nil
	}
	select {
	case <-this.stop:
	default:
		close(this.stop)
	}
	return nil
}

func (this *Memory) worker() {
	ticker := time.NewTicker(time.Second * time.Duration(Heartbeat))
	defer ticker.Stop()
	for {
		select {
		case <-this.stop:
			return
		case <-ticker.C:
			this.clean()
		}
	}
}

func (this *Memory) clean() {
	nowTime := time.Now().Unix()
	var remove []uint64
	this.Cache.Range(func(item cache.Interface) bool {
		if data, ok := item.(*Setter); ok && data.expire < nowTime {
			remove = append(remove, item.Id())
		}
		return true
	})
	if len(remove) > 0 {
		this.Cache.Remove(remove...)
	}
}
