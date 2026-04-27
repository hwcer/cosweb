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
