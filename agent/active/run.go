package active

import (
	"fmt"
	"log"
	"mosona-manager/agent/active/middleware"
	"mosona-manager/agent/config"
	"net/http"
)

func Run() {
	if err := config.LoadPublicKey(); err != nil {
		log.Fatalln("Failed to load public key:", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/ws/state", middleware.WsMiddleware(handleStateWebSocket))
	mux.HandleFunc("/api/ws/terminal", middleware.WsMiddleware(handleTerminalWebSocket))
	mux.HandleFunc("/api/info", handleInfo)

	handler := middleware.AuthMiddleware(mux)

	addr := fmt.Sprintf("%s:%d", config.Current.Host, config.Current.Port)
	log.Printf("Listening on %s...\n", handler)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalln("Failed to start HTTP server:", err)
	}
}
