package active

import (
	"crypto/ed25519"
	"fmt"
	"log"
	"mosona-manager/agent/active/middleware"
	"mosona-manager/agent/config"
	"mosona-manager/pkg/identity"
	"net/http"
	"time"
)

const (
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 60 * time.Second
)

func Run() {
	if err := config.LoadPublicKey(); err != nil {
		log.Fatalln("Failed to load public key:", err)
	}
	if err := config.LoadOrCreateActivePrivateKey(); err != nil {
		log.Fatalln("Failed to load active identity:", err)
	}
	if fingerprint, err := identity.Ed25519Fingerprint(config.PrivateKey.Public().(ed25519.PublicKey)); err == nil {
		log.Println("Active Agent identity fingerprint:", fingerprint)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/ws/pair", middleware.WsMiddleware(handlePairWebSocket))
	mux.HandleFunc("/api/ws/state", middleware.WsMiddleware(handleStateWebSocket))
	if !config.Current.NoTerminal {
		mux.HandleFunc("/api/ws/terminal", middleware.WsMiddleware(handleTerminalWebSocket))
	}
	mux.HandleFunc("/api/info", handleInfo)

	handler := middleware.AuthMiddleware(mux)

	server := newHTTPServer(fmt.Sprintf("%s:%d", config.Current.Host, config.Current.Port), handler)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalln("Failed to start HTTP server:", err)
	}
}

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
}
