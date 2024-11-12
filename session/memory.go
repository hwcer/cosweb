package session

import (
	"github.com/hwcer/cosgo/storage"
	"time"
)

var Heartbeat int32 = 10 //心跳(S)

func NewMemory() *Memory {
	s := &Memory{
		Array: *storage.New(1024),
	}
	s.Array.NewSetter = NewSetter
	return s
}

type Memory struct {
	storage.Array
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
	if v, ok := this.Array.Get(mid); !ok {
		return nil, ErrorSessionIllegal
	} else {
		return v.(*Setter), nil
	}
}

func (this *Memory) Verify(token string) (p *Player, err error) {
	var setter *Setter
	if setter, err = this.get(token); err == nil {
		p, _ = setter.Get().(*Player)
	}
	return
}

// Update 更新信息，内存没事，共享Player信息已经更新过，仅仅设置过去时间
func (this *Memory) Update(p *Player, data map[string]any, ttl int64) (err error) {
	var setter *Setter
	setter, err = this.get(p.token)
	if err != nil {
		return
	}
	if ttl > 0 {
		setter.Expire(ttl)
	}
	return
}
func (this *Memory) Delete(p *Player) error {
	mid := storage.MID(p.token)
	this.Array.Delete(mid)
	return nil
}

// Create 创建新SESSION,返回SESSION Index
// Create会自动设置有效期
// Create新数据为锁定状态
func (this *Memory) Create(uuid string, data map[string]any, ttl int64) (p *Player, err error) {
	d := this.Array.Create(nil)
	setter, _ := d.(*Setter)
	st := string(setter.Id())
	p = NewPlayer(uuid, st, data)
	setter.Set(p)
	if ttl > 0 {
		setter.Expire(ttl)
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
	var keys []storage.MID
	this.Array.Range(func(item storage.Setter) bool {
		if data, ok := item.(*Setter); ok && data.expire < nowTime {
			keys = append(keys, data.Id())
		}
		return true
	})
	if len(keys) > 0 {
		this.Remove(keys...)
	}
}
