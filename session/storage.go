package session

//type Data interface {
//	Get(key string) any
//	Set(key string, val any) any
//	Merge(values map[string]any, replace bool)
//	Range(func(k string, v any) bool)
//}

type Storage interface {
	Verify(token string) (player *Player, err error)                                //验证TOKEN信息
	Create(uuid string, data map[string]any, ttl int64) (player *Player, err error) //用户登录创建新session
	Update(player *Player, data map[string]any, ttl int64) error                    //更新session数据
	Delete(player *Player) error                                                    //退出登录删除SESSION
	Start() error                                                                   //启动服务器时初始化SESSION Storage
	Close() error                                                                   //关闭服务器时断开连接等
}

func Start(s Storage) error {
	if s != nil {
		Options.storage = s
	} else {
		Options.storage = NewMemory()
	}
	return Options.storage.Start()
}

func Close() error {
	if Options.storage != nil {
		return Options.storage.Close()
	} else {
		return nil
	}
}
