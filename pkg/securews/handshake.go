package secureWS

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
)

// ProtocolVersion is the authenticated Active Agent handshake version.
const ProtocolVersion uint8 = 2

const (
	// HubFinished is the Hub key-confirmation plaintext.
	HubFinished = "mosona-secure-ws-v2 hub finished"
	// AgentFinished is the Agent key-confirmation plaintext.
	AgentFinished = "mosona-secure-ws-v2 agent finished"
)

// HandshakeContext binds a handshake to its HTTP upgrade request.
type HandshakeContext struct {
	AgentUID      string
	Path          string
	HTTPNonce     string
	HTTPTimestamp string
}

// HandshakeTranscript contains every value authenticated by the Agent.
type HandshakeTranscript struct {
	Context         HandshakeContext
	HubX25519Pub    []byte
	HubNonce        []byte
	HubTimestamp    int64
	AgentEd25519Pub []byte
	AgentX25519Pub  []byte
	AgentNonce      []byte
}

// HubSignatureMessage returns the message signed by the Hub identity key.
func HubSignatureMessage(ctx HandshakeContext, hubX25519Pub, hubNonce []byte, timestamp int64) []byte {
	return hashFields(
		[]byte("mosona-secure-ws-v2 hub signature"),
		[]byte{ProtocolVersion}, []byte(ctx.AgentUID), []byte(ctx.Path),
		[]byte(ctx.HTTPNonce), []byte(ctx.HTTPTimestamp), hubX25519Pub,
		hubNonce, int64Bytes(timestamp),
	)
}

// Hash returns the canonical digest of the authenticated handshake transcript.
func (t HandshakeTranscript) Hash() []byte {
	return hashFields(
		[]byte("mosona-secure-ws-v2 transcript"),
		[]byte{ProtocolVersion}, []byte(t.Context.AgentUID), []byte(t.Context.Path),
		[]byte(t.Context.HTTPNonce), []byte(t.Context.HTTPTimestamp),
		t.HubX25519Pub, t.HubNonce, int64Bytes(t.HubTimestamp),
		t.AgentEd25519Pub, t.AgentX25519Pub, t.AgentNonce,
	)
}

// AgentSignatureMessage returns the message signed by the Agent identity key.
func AgentSignatureMessage(transcriptHash []byte) []byte {
	return hashFields([]byte("mosona-secure-ws-v2 agent signature"), transcriptHash)
}

// DecodeExactBase64 strictly decodes a standard Base64 value of the requested size.
func DecodeExactBase64(value string, size int) ([]byte, error) {
	b, err := base64.StdEncoding.Strict().DecodeString(value)
	if err != nil || len(b) != size || base64.StdEncoding.EncodeToString(b) != value {
		return nil, fmt.Errorf("invalid encoded handshake field")
	}
	return b, nil
}

func hashFields(fields ...[]byte) []byte {
	h := sha256.New()
	var length [8]byte
	for _, field := range fields {
		binary.BigEndian.PutUint64(length[:], uint64(len(field)))
		_, _ = h.Write(length[:])
		_, _ = h.Write(field)
	}
	return h.Sum(nil)
}

func int64Bytes(value int64) []byte {
	var result [8]byte
	binary.BigEndian.PutUint64(result[:], uint64(value))
	return result[:]
}
