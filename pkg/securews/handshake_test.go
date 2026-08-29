package secureWS

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func TestDecodeExactBase64IsStrict(t *testing.T) {
	valid := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 16))
	if _, err := DecodeExactBase64(valid, 16); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{valid + "\n", valid[:len(valid)-1] + "!", base64.StdEncoding.EncodeToString([]byte("short"))} {
		if _, err := DecodeExactBase64(value, 16); err == nil {
			t.Fatalf("DecodeExactBase64(%q) succeeded", value)
		}
	}
}

func TestHandshakeTranscriptBindsEveryField(t *testing.T) {
	base := HandshakeTranscript{
		Context:      HandshakeContext{AgentUID: "agent", Path: "/api/ws/terminal", HTTPNonce: "nonce", HTTPTimestamp: "123"},
		HubX25519Pub: bytes.Repeat([]byte{1}, 32), HubNonce: bytes.Repeat([]byte{2}, 16), HubTimestamp: 123,
		AgentEd25519Pub: bytes.Repeat([]byte{3}, 32), AgentX25519Pub: bytes.Repeat([]byte{4}, 32), AgentNonce: bytes.Repeat([]byte{5}, 16),
	}
	want := base.Hash()
	tests := []struct {
		name   string
		mutate func(*HandshakeTranscript)
	}{
		{name: "AgentUID", mutate: func(t *HandshakeTranscript) { t.Context.AgentUID = "other-agent" }},
		{name: "Path", mutate: func(t *HandshakeTranscript) { t.Context.Path = "/api/ws/state" }},
		{name: "HTTPNonce", mutate: func(t *HandshakeTranscript) { t.Context.HTTPNonce = "other-http-nonce" }},
		{name: "HTTPTimestamp", mutate: func(t *HandshakeTranscript) { t.Context.HTTPTimestamp = "456" }},
		{name: "HubX25519Pub", mutate: func(t *HandshakeTranscript) { t.HubX25519Pub = bytes.Repeat([]byte{6}, 32) }},
		{name: "HubNonce", mutate: func(t *HandshakeTranscript) { t.HubNonce = bytes.Repeat([]byte{7}, 16) }},
		{name: "HubTimestamp", mutate: func(t *HandshakeTranscript) { t.HubTimestamp++ }},
		{name: "AgentEd25519Pub", mutate: func(t *HandshakeTranscript) { t.AgentEd25519Pub = bytes.Repeat([]byte{8}, 32) }},
		{name: "AgentX25519Pub", mutate: func(t *HandshakeTranscript) { t.AgentX25519Pub = bytes.Repeat([]byte{9}, 32) }},
		{name: "AgentNonce", mutate: func(t *HandshakeTranscript) { t.AgentNonce = bytes.Repeat([]byte{10}, 16) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := base
			test.mutate(&changed)
			if bytes.Equal(want, changed.Hash()) {
				t.Fatalf("%s mutation did not change transcript hash", test.name)
			}
		})
	}
}

func TestSessionCryptoV2RequiresMatchingTranscript(t *testing.T) {
	curve := ecdh.X25519()
	hub, _ := curve.GenerateKey(rand.Reader)
	agent, _ := curve.GenerateKey(rand.Reader)
	hash := bytes.Repeat([]byte{7}, 32)
	hubCrypto, err := NewSessionCryptoV2(RoleHub, agent.PublicKey(), hub, hash)
	if err != nil {
		t.Fatal(err)
	}
	agentCrypto, err := NewSessionCryptoV2(RoleAgent, hub.PublicKey(), agent, hash)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := hubCrypto.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := agentCrypto.Decrypt(frame)
	if err != nil || string(plain) != "secret" {
		t.Fatalf("decrypt = %q, %v", plain, err)
	}

	wrongCrypto, err := NewSessionCryptoV2(RoleAgent, hub.PublicKey(), agent, bytes.Repeat([]byte{8}, 32))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = wrongCrypto.Decrypt(frame); err == nil {
		t.Fatal("mismatched transcript decrypted a frame")
	}
}
