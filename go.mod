module github.com/hwcer/cosweb

go 1.17

replace (
	github.com/hwcer/cosgo v0.0.0 => ../cosgo
	github.com/hwcer/cosmo v0.0.0 => ../cosmo
	github.com/hwcer/cosweb v0.0.0 => ./
	github.com/hwcer/registry v0.0.0 => ../registry
)

require (
	github.com/hwcer/cosgo v0.0.0
	github.com/hwcer/cosmo v0.0.0
	github.com/hwcer/registry v0.0.0
)

require (
	github.com/StackExchange/wmi v0.0.0-20190523213315-cbe66965904d // indirect
	github.com/coreos/etcd v3.3.13+incompatible // indirect
	github.com/coreos/go-systemd v0.0.0-20190719114852-fd7a80b32e1f // indirect
	github.com/coreos/pkg v0.0.0-20180928190104-399ea9e2e55f // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fsnotify/fsnotify v1.5.1 // indirect
	github.com/go-ole/go-ole v1.2.4 // indirect
	github.com/go-redis/redis v6.15.9+incompatible
	github.com/go-stack/stack v1.8.0 // indirect
	github.com/golang/protobuf v1.5.2
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/jinzhu/now v1.1.4 // indirect
	github.com/magiconair/properties v1.8.1 // indirect
	github.com/mitchellh/mapstructure v1.4.2 // indirect
	github.com/onsi/gomega v1.19.0 // indirect
	github.com/pelletier/go-toml v1.7.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/shirou/gopsutil v2.20.8+incompatible // indirect
	github.com/spf13/afero v1.1.2 // indirect
	github.com/spf13/cast v1.3.0 // indirect
	github.com/spf13/jwalterweatherman v1.0.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/spf13/viper v1.7.1 // indirect
	github.com/stretchr/testify v1.7.0
	github.com/subosito/gotenv v1.2.0 // indirect
	github.com/ugorji/go/codec v1.1.7
	go.mongodb.org/mongo-driver v1.9.1 // indirect
	golang.org/x/crypto v0.0.0-20211117183948-ae814b36b871
	golang.org/x/net v0.0.0-20220225172249-27dd8689420f // indirect
	golang.org/x/sys v0.0.0-20211216021012-1d35b9e2eb4e // indirect
	golang.org/x/text v0.3.7 // indirect
	google.golang.org/protobuf v1.27.1 // indirect
	gopkg.in/ini.v1 v1.51.0 // indirect
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
)
