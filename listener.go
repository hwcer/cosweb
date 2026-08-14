package cosweb

import (
	"crypto/tls"
	"net"
	"sync"
)

var (
	makeListenersMu sync.RWMutex
	makeListeners   = map[string]MakeListener{}
)

func init() {
	tcp := tcpMakeListener("tcp")
	makeListeners["tcp"] = tcp
	makeListeners["tcp4"] = tcpMakeListener("tcp4")
	makeListeners["tcp6"] = tcpMakeListener("tcp6")
	makeListeners["http"] = tcp
	makeListeners["ws"] = tcp
	makeListeners["wss"] = tcp
}

// MakeListener defines a listener generator.
type MakeListener func(address string, tlsConfig *tls.Config) (ln net.Listener, err error)

// RegisterListener registers a MakeListener for network.
func RegisterListener(network string, ml MakeListener) {
	makeListenersMu.Lock()
	defer makeListenersMu.Unlock()
	makeListeners[network] = ml
}

func tcpMakeListener(network string) MakeListener {
	return func(address string, tlsConfig *tls.Config) (ln net.Listener, err error) {
		if tlsConfig == nil {
			ln, err = net.Listen(network, address)
		} else {
			ln, err = tls.Listen(network, address, tlsConfig)
		}
		return ln, err
	}
}

func Listener(network string) MakeListener {
	makeListenersMu.RLock()
	defer makeListenersMu.RUnlock()
	return makeListeners[network]
}

// Endpoint 描述一个监听端点。
type Endpoint struct {
	Network string      //为空时默认 tcp,其他取值查 makeListeners 注册表
	Address string      //:8080 / 127.0.0.1:9090 / :0
	TLS     *tls.Config //nil 为明文
}

// withALPN 在调用方未指定 ALPN 时补齐 h2/http1.1。
// 手工包装 TLS listener 后 http.Server.Serve 不会像 ListenAndServeTLS 那样自动追加 "h2",
// 不补的话 TLSConfigParse 产出的配置会退化成 http/1.1 only。
// NextProtos 非空说明调用方有明确意图(例如只要 http/1.1),原样返回。
// 克隆后再改,不污染调用方传入的 tls.Config。
func withALPN(cfg *tls.Config) *tls.Config {
	if cfg == nil || len(cfg.NextProtos) > 0 {
		return cfg
	}
	c := cfg.Clone()
	c.NextProtos = []string{"h2", "http/1.1"}
	return c
}
