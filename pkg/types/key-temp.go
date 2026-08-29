package pbTypes

type KTHub struct {
	Version      uint8  `msgpack:"version"`
	HubX25519Pub string `msgpack:"hub_x25519_pub"`
	HubNonce     string `msgpack:"hub_nonce"`
	Timestamp    int64  `msgpack:"timestamp"`
	Sign         string `msgpack:"sign"`
}

type KTAgent struct {
	Version         uint8  `msgpack:"version"`
	AgentX25519Pub  string `msgpack:"agent_x25519_pub"`
	AgentNonce      string `msgpack:"agent_nonce"`
	AgentEd25519Pub string `msgpack:"agent_ed25519_pub"`
	Sign            string `msgpack:"sign"`
}
