package secureWS

type SecureFrame struct {
	Seq        uint64 `msgpack:"seq"`
	Ciphertext []byte `msgpack:"ciphertext"`
}
