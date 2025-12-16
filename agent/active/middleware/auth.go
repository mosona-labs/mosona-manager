package middleware

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"mosona-manager/agent/config"
	"net/http"
)

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid := r.Header.Get("X-Agent-Id")
		ts := r.Header.Get("X-Agent-Timestamp")
		nonce := r.Header.Get("X-Agent-Nonce")
		signature := r.Header.Get("X-Agent-Signature")
		if uid == "" || ts == "" || len(nonce) > 16 || signature == "" {
			http.Error(w, "Missing authentication headers", http.StatusUnauthorized)
			return
		}
		if uid != config.Current.Uid {
			http.Error(w, "Invalid agent ID", http.StatusUnauthorized)
			return
		}

		sig, err := base64.StdEncoding.DecodeString(signature)
		if err != nil {
			http.Error(w, "Invalid signature encoding", http.StatusUnauthorized)
			return
		}

		if !ed25519.Verify(config.PublicKey, []byte(fmt.Sprintf("%s\n%s\n%s", uid, ts, nonce)), sig) {
			http.Error(w, "Signature verification failed", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
