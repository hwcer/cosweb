package session

import "github.com/hwcer/cosgo/values"

var storage Storage

type Storage interface {
	Get(token string, lock bool) (uuid string, data values.Values, err error)               //获取session镜像数据
	Save(uuid string, data values.Values, ttl int64, unlock bool) error                     //设置(覆盖)session数据
	Create(uuid string, data values.Values, ttl int64, lock bool) (token string, err error) //用户登录创建新session
	Delete(uuid string) error                                                               //退出登录删除SESSION
	Start() error                                                                           //启动服务器时初始化SESSION Storage
	Close() error                                                                           //关闭服务器时断开连接等
}

//func Set(s Storage) {
//	storage = s
//}
//
//func Get() Storage {
//	return storage
//}

func Start(s Storage) error {
	storage = s
	//storage, _ = NewRedis("127.0.0.1")
	return storage.Start()
}

func Close() error {
	if storage != nil {
		return storage.Close()
	} else {
		return nil
	}
}
