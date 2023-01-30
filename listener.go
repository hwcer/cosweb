package cosweb

import (
	"crypto/tls"
	"net"
)

var makeListeners = make(map[string]MakeListener)

func init() {
	makeListeners["tcp"] = tcpMakeListener("tcp")
	makeListeners["tcp4"] = tcpMakeListener("tcp4")
	makeListeners["tcp6"] = tcpMakeListener("tcp6")
	makeListeners["http"] = tcpMakeListener("tcp")
	makeListeners["ws"] = tcpMakeListener("tcp")
	makeListeners["wss"] = tcpMakeListener("tcp")
}

// MakeListener defines a listener generator.
type MakeListener func(address string, tlsConfig *tls.Config) (ln net.Listener, err error)

// RegisterListener registers a MakeListener for network.
func RegisterListener(network string, ml MakeListener) {
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
	return makeListeners[network]
}
