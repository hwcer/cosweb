package session

import (
	"context"
	"errors"
	"github.com/hwcer/cosgo/redis"
	"github.com/hwcer/cosgo/values"
	"strings"
	"time"
)

const redisSessionKeyUid = "_sess_uid"
const redisSessionKeyLock = "_sess_lock"

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

func (this *Redis) lock(rkey string, data values.Values) bool {
	if data != nil {
		if v := data.GetInt64(redisSessionKeyLock); v > 0 {
			return false
		}
	}
	if v, err := this.client.HIncrBy(context.Background(), rkey, redisSessionKeyLock, 1).Result(); err != nil || v > 1 {
		return false
	}
	return true
}

// Get 获取session镜像数据
func (this *Redis) Get(token string, lock bool) (uuid string, data values.Values, err error) {
	if uuid, err = Decode(token); err != nil {
		return
	}
	var val map[string]string
	rkey := this.rkey(uuid)
	if val, err = this.client.HGetAll(context.Background(), rkey).Result(); err != nil {
		return
	}
	if v, ok := val[redisSessionKeyUid]; !ok {
		return "", nil, ErrorSessionNotExist
	} else if v != uuid {
		return "", nil, ErrorSessionIllegal
	}

	data = make(values.Values, len(val))
	for k, v := range val {
		data.Set(k, v)
	}

	if lock && !this.lock(rkey, data) {
		return "", nil, ErrorSessionLocked
	}
	return
}

// Create ttl过期时间(s)
func (this *Redis) Create(uuid string, data values.Values, ttl int64, lock bool) (token string, err error) {
	rkey := this.rkey(uuid)
	if lock {
		data.Set(redisSessionKeyLock, 1)
	} else {
		data.Set(redisSessionKeyLock, 0)
	}
	data[redisSessionKeyUid] = uuid
	args := make([]interface{}, 0, len(data)*2)
	for k, v := range data {
		args = append(args, k, v)
	}

	if err = this.client.HMSet(context.Background(), rkey, args...).Err(); err != nil {
		return
	}
	if ttl > 0 {
		this.client.Expire(context.Background(), rkey, time.Duration(ttl)*time.Second)
	}
	token, err = Encode(uuid)
	return

}
func (this *Redis) Save(token string, data values.Values, ttl int64, unlock bool) (err error) {
	var uuid string
	if uuid, err = Decode(token); err != nil {
		return
	}
	rkey := this.rkey(uuid)
	//pipeline := this.client.Pipeline()
	if unlock {
		data[redisSessionKeyLock] = int64(0)
	}

	if len(data) > 0 {
		args := make([]interface{}, 0, len(data)*2)
		for k, v := range data {
			args = append(args, k, v)
		}
		if _, err = this.client.HMSet(context.Background(), rkey, args...).Result(); err != nil {
			return
		}
	}
	if ttl > 0 {
		this.client.Expire(context.Background(), rkey, time.Duration(ttl)*time.Second)
	}

	return
}

func (this *Redis) Delete(uuid string) (err error) {
	rkey := this.rkey(uuid)
	_, err = this.client.Del(context.Background(), rkey).Result()
	return
}
