package session

import (
	"context"
	"errors"
	"github.com/hwcer/cosgo/redis"
	"strings"
	"time"
)

const redisSessionKeyTokenName = "_cookie_key_token"

//const redisSessionKeyLock = "_sess_lock"

type Redis struct {
	prefix  []string
	client  *redis.Client
	address string
}

func NewRedis(address interface{}, prefix ...string) (c *Redis, err error) {
	c = &Redis{
		prefix: prefix,
	}
	c.prefix = append(c.prefix, "cookie")

	switch v := address.(type) {
	case *redis.Client:
		c.client = v
	case string:
		c.address = v
	default:
		err = errors.New("address type must be string or *redis.Client")
	}
	return
}

func (this *Redis) Start() (err error) {
	if this.client != nil {
		return
	}
	this.client, err = redis.New(this.address)
	return
}

func (this *Redis) Close() error {
	if this.address == "" {
		return nil
	}
	return this.client.Close()
}

func (this *Redis) rkey(uuid string) string {
	return strings.Join(append(this.prefix, uuid), "-")
}

//func (this *Redis) lock(rkey string, data values.Values) bool {
//	if data != nil {
//		if v := data.GetInt64(redisSessionKeyLock); v > 0 {
//			return false
//		}
//	}
//	if v, err := this.client.HIncrBy(context.Background(), rkey, redisSessionKeyLock, 1).Result(); err != nil || v > 1 {
//		return false
//	}
//	return true
//}

// Verify 获取session镜像数据
func (this *Redis) Verify(token string) (p *Player, err error) {
	var uuid string
	if uuid, err = Decode(token); err != nil {
		return
	}
	val := map[string]string{}
	rk := this.rkey(uuid)
	if val, err = this.client.HGetAll(context.Background(), rk).Result(); err != nil {
		return
	}
	if v, ok := val[redisSessionKeyTokenName]; !ok {
		return nil, ErrorSessionNotExist
	} else if v != token {
		return nil, ErrorSessionIllegal
	}
	data := map[string]any{}
	for k, v := range val {
		data[k] = v
	}
	p = NewPlayer(uuid, token, data)
	return
}

// Create ttl过期时间(s)
func (this *Redis) Create(uuid string, data map[string]any, ttl int64) (p *Player, err error) {
	rk := this.rkey(uuid)
	var st string
	if st, err = Encode(uuid); err == nil {
		return
	}
	data[redisSessionKeyTokenName] = st
	var args []any
	for k, v := range data {
		args = append(args, k, v)
	}
	if err = this.client.HMSet(context.Background(), rk, args...).Err(); err != nil {
		return
	}
	if ttl > 0 {
		this.client.Expire(context.Background(), rk, time.Duration(ttl)*time.Second)
	}
	p = NewPlayer(uuid, st, data)
	return
}

func (this *Redis) Update(p *Player, data map[string]any, ttl int64) (err error) {
	var uuid string
	if uuid, err = Decode(p.token); err != nil {
		return
	}
	rk := this.rkey(uuid)
	//pipeline := this.client.Pipeline()
	if len(data) > 0 {
		args := make([]any, 0, len(data)*2)
		for k, v := range data {
			args = append(args, k, v)
		}
		if _, err = this.client.HMSet(context.Background(), rk, args...).Result(); err != nil {
			return
		}
	}
	if ttl > 0 {
		this.client.Expire(context.Background(), rk, time.Duration(ttl)*time.Second)
	}

	return
}

func (this *Redis) Delete(p *Player) (err error) {
	rk := this.rkey(p.uuid)
	_, err = this.client.Del(context.Background(), rk).Result()
	return
}
