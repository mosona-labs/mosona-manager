package active

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"strings"
	"testing"

	secureWS "mosona-manager/pkg/securews"
)

type singleStateMessageReader struct {
	data []byte
}

func (r *singleStateMessageReader) ReadMessage() (int, []byte, error) {
	return 2, r.data, nil
}

func TestReadActiveAgentStatusesReturnsOnDecryptFailure(t *testing.T) {
	curve := ecdh.X25519()
	hubPrivate, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	agentPrivate, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	transcriptHash := []byte("test transcript hash")
	hubCrypto, err := secureWS.NewSessionCryptoV2(secureWS.RoleHub, agentPrivate.PublicKey(), hubPrivate, transcriptHash)
	if err != nil {
		t.Fatal(err)
	}
	agentCrypto, err := secureWS.NewSessionCryptoV2(secureWS.RoleAgent, hubPrivate.PublicKey(), agentPrivate, transcriptHash)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := agentCrypto.Encrypt([]byte("status"))
	if err != nil {
		t.Fatal(err)
	}
	frame[len(frame)-1] ^= 0xff

	err = readActiveAgentStatuses(context.Background(), &singleStateMessageReader{data: frame}, hubCrypto, 91)
	if err == nil || !strings.Contains(err.Error(), "decrypt Active Agent status") {
		t.Fatalf("readActiveAgentStatuses() error = %v, want decrypt failure", err)
	}
}
