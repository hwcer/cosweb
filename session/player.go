package session

import (
	"github.com/hwcer/cosgo/values"
	"sync"
)

func NewPlayer(uuid string, token string, vs map[string]any) *Player {
	p := &Player{uuid: uuid, token: token}
	if len(vs) > 0 {
		p.Values = vs
	} else {
		p.Values = values.Values{}
	}
	return p
}

// Player 用户登录信息,不要直接修改 Player.Values 信息
type Player struct {
	uuid  string //GUID
	token string
	sync.Mutex
	values.Values //用户登录信息,推荐存入一个struct
}

func (this *Player) Set(key string, value any) any {
	this.Lock()
	defer this.Unlock()
	vs := this.Values.Clone()
	vs.Set(key, value)
	this.Values = vs
	return value
}

func (this *Player) Merge(p *Player) {
	if this.token == p.token {
		return
	}
	this.Lock()
	defer this.Unlock()
	this.uuid = p.uuid
	this.token = p.token
	vs := this.Values.Clone()
	vs.Merge(p.Values, true)
	this.Values = vs
}

// Update 批量设置Cookie信息
func (this *Player) Update(data map[string]any) {
	this.Lock()
	defer this.Unlock()
	vs := this.Values.Clone()
	for k, v := range data {
		vs.Set(k, v)
	}
	this.Values = vs
}

func (this *Player) UUID() string {
	return this.uuid
}

func (this *Player) Token() string {
	return this.token
}
