package middleware

import (
	"mosona-manager/agent/config"
	"mosona-manager/pkg/identity"
	"net/http"
	"time"
)

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid := r.Header.Get("X-Agent-Id")
		ts := r.Header.Get("X-Agent-Timestamp")
		nonce := r.Header.Get("X-Agent-Nonce")
		signature := r.Header.Get("X-Agent-Signature")
		if uid == "" || ts == "" || len(nonce) < 16 || signature == "" {
			http.Error(w, "Missing authentication headers", http.StatusUnauthorized)
			return
		}
		if uid != config.Current.Uid {
			http.Error(w, "Invalid agent ID", http.StatusUnauthorized)
			return
		}

		if err := identity.VerifySignedHeaders(config.PublicKey, uid, ts, nonce, signature, time.Now()); err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
