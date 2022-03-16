// Copyright 2017 Manu Martinez-Almeida.  All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package binding

import (
	"bytes"
	"github.com/ugorji/go/codec"
	"io"
)

func init() {
	b := protobufBinding{}
	Register(MIMEMSGPACK, b)
	Register(MIMEMSGPACK2, b)
}

type msgpackBinding struct{}

func (msgpackBinding) Name() string {
	return "msgpack"
}

func (msgpackBinding) Bind(body io.Reader, obj interface{}) error {
	return decodeMsgPack(body, obj)
}

func (msgpackBinding) BindBody(body []byte, obj interface{}) error {
	return decodeMsgPack(bytes.NewReader(body), obj)
}

func decodeMsgPack(r io.Reader, obj interface{}) error {
	cdc := new(codec.MsgpackHandle)
	return codec.NewDecoder(r, cdc).Decode(&obj)
}
