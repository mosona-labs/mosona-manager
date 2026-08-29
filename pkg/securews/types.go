package secureWS

// SecureFrame carries one ordered encrypted session payload.
type SecureFrame struct {
	Seq        uint64 `msgpack:"seq"`
	Ciphertext []byte `msgpack:"ciphertext"`
}
