package main

import (
	"github.com/hwcer/cosweb"
)

func main() {
	Server := cosweb.NewServer()
	Server.Register("/ping", ping)
	if err := Server.Run(":80"); err != nil {
		panic(err)
	}
}

func ping(context *cosweb.Context, next cosweb.Next) error {
	return context.String("ok")
}
