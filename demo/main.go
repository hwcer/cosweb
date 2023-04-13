package main

import (
	"github.com/hwcer/cosgo"
	"github.com/hwcer/cosweb"
)

var Server = cosweb.NewServer()

func main() {
	cosgo.Start(true, &module{Module: *cosgo.NewModule("http server")})
}

func ping(context *cosweb.Context, next cosweb.Next) error {
	return context.String("ok")
}

type module struct {
	cosgo.Module
}

func (m *module) Start() error {
	Server.Register("/ping", ping)
	return Server.Start(":80")
}
func (m *module) Close() error {
	return Server.Close()
}
