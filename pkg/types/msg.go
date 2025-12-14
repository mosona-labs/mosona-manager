package pbTypes

type Msg struct {
	Code string `msgpack:"code"`
	Data []byte `msgpack:"data"`
}
