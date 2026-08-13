package middleware

import (
	"encoding/base64"
	"mosona-manager/agent/config"
	pbTypes "mosona-manager/pkg/types"
	"strings"
	"testing"
	"time"
)

func TestVerifyHandshakeInitRejectsInvalidConfiguredPublicKeyLength(t *testing.T) {
	oldPublicKey := config.PublicKey
	config.PublicKey = []byte("short")
	t.Cleanup(func() { config.PublicKey = oldPublicKey })

	message := &pbTypes.KTHub{
		HubX25519Pub: base64.StdEncoding.EncodeToString(make([]byte, 32)),
		HubNonce:     base64.StdEncoding.EncodeToString(make([]byte, 16)),
		Timestamp:    time.Now().Unix(),
		Sign:         base64.StdEncoding.EncodeToString(make([]byte, 64)),
	}
	if err := verifyHandshakeInit(message, time.Now()); err == nil || !strings.Contains(err.Error(), "public key length") {
		t.Fatalf("verifyHandshakeInit() error = %v, want public key length error", err)
	}
}
