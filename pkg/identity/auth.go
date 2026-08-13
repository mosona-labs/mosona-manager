package identity

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"strconv"
	"sync"
	"time"
)

const SignedHeaderMaxSkew = 120 * time.Second

var signedHeaderReplay = struct {
	sync.Mutex
	seen map[string]time.Time
}{
	seen: make(map[string]time.Time),
}

func VerifySignedHeaders(publicKey ed25519.PublicKey, uid, ts, nonce, signature string, now time.Time) error {
	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid public key length")
	}
	tsUnix, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp")
	}
	t := time.Unix(tsUnix, 0)
	if t.After(now.Add(SignedHeaderMaxSkew)) || t.Before(now.Add(-SignedHeaderMaxSkew)) {
		return fmt.Errorf("timestamp out of range")
	}

	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("invalid signature encoding")
	}
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("invalid signature length")
	}

	msg := fmt.Sprintf("%s\n%s\n%s", uid, ts, nonce)
	if !ed25519.Verify(publicKey, []byte(msg), sig) {
		return fmt.Errorf("signature verification failed")
	}

	if isReplay(uid, ts, nonce, now) {
		return fmt.Errorf("replayed authentication nonce")
	}
	return nil
}

func isReplay(uid, ts, nonce string, now time.Time) bool {
	key := uid + "\x00" + ts + "\x00" + nonce
	expiresAt := now.Add(SignedHeaderMaxSkew)

	signedHeaderReplay.Lock()
	defer signedHeaderReplay.Unlock()

	for k, expiry := range signedHeaderReplay.seen {
		if !expiry.After(now) {
			delete(signedHeaderReplay.seen, k)
		}
	}

	if _, ok := signedHeaderReplay.seen[key]; ok {
		return true
	}
	signedHeaderReplay.seen[key] = expiresAt
	return false
}
