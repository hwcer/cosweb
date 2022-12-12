package session

import (
	"github.com/hwcer/cosgo/storage"
	"github.com/hwcer/cosgo/values"
	"time"
)

var Heartbeat int32 = 10 //心跳(S)

func NewMemory() *Memory {
	s := &Memory{
		Hash: *storage.NewHash(1024),
	}
	s.Array.NewSetter = NewSetter
	return s
}

type Memory struct {
	storage.Hash
	stop chan struct{}
}

func (this *Memory) Start() error {
	if Options.MaxAge > 0 {
		this.stop = make(chan struct{})
		go this.worker()
	}
	return nil
}

func (this *Memory) get(token string) (*Setter, error) {
	mid := storage.MID(token)
	if v, ok := this.Hash.Array.Get(mid); !ok {
		return nil, ErrorSessionIllegal
	} else {
		data, _ := v.(*Setter)
		return data, nil
	}
}

func (this *Memory) Get(token string, lock bool) (uuid string, result values.Values, err error) {
	var data *Setter
	data, err = this.get(token)
	if err != nil {
		return
	}
	if lock && !data.Lock() {
		err = ErrorSessionLocked
		return
	}
	uuid = data.uuid
	vs, _ := data.Get().(values.Values)
	result = make(values.Values)
	for k, v := range vs {
		result.Set(k, v)
	}
	return
}

func (this *Memory) Save(token string, data values.Values, ttl int64, unlock bool) (err error) {
	var setter *Setter
	setter, err = this.get(token)
	if err != nil {
		return
	}

	vs, _ := setter.Get().(values.Values)
	if vs == nil {
		return ErrorSessionTypeError
	}

	newData := make(values.Values)
	for k, v := range vs {
		newData.Set(k, v)
	}
	for k, v := range data {
		newData.Set(k, v)
	}
	setter.Set(newData)

	if ttl > 0 {
		setter.Expire(ttl)
	}
	if unlock {
		setter.UnLock()
	}
	return
}
func (this *Memory) Delete(uuid string) error {
	this.Hash.Delete(uuid)
	return nil
}

// Create 创建新SESSION,返回SESSION Index
// Create会自动设置有效期
// Create新数据为锁定状态
func (this *Memory) Create(uuid string, data values.Values, ttl int64, lock bool) (token string, err error) {
	d := this.Hash.Create(uuid, data)
	setter, _ := d.(*Setter)
	setter.uuid = uuid
	token = string(setter.Id())
	if ttl > 0 {
		setter.Expire(ttl)
	}
	if !lock {
		setter.UnLock() //默认加锁
	}
	return
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
	var keys []string
	this.Hash.Array.Range(func(item storage.Setter) bool {
		if data, ok := item.(*Setter); ok && data.expire < nowTime {
			keys = append(keys, data.uuid)
		}
		return true
	})
	if len(keys) > 0 {
		this.Hash.Remove(keys...)
	}
}
