package session

type Data interface {
	Get(key string) any
	Set(key string, val any) any
	Merge(values map[string]any, replace bool)
}

type Storage interface {
	Get(token string, lock bool) (uuid string, data Data, err error)               //获取session镜像数据
	Save(uuid string, data map[string]any, ttl int64, unlock bool) error           //设置(覆盖)session数据
	Create(uuid string, data Data, ttl int64, lock bool) (token string, err error) //用户登录创建新session
	Delete(uuid string) error                                                      //退出登录删除SESSION
	Start() error                                                                  //启动服务器时初始化SESSION Storage
	Close() error                                                                  //关闭服务器时断开连接等
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
