package active

import (
	"net/http"
	"testing"
)

func TestNewHTTPServerConfiguresTimeouts(t *testing.T) {
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	server := newHTTPServer("127.0.0.1:1234", handler)

	if server.Addr != "127.0.0.1:1234" || server.Handler == nil {
		t.Fatalf("server address or handler not configured")
	}
	if server.ReadHeaderTimeout != readHeaderTimeout || server.ReadTimeout != readTimeout ||
		server.WriteTimeout != writeTimeout || server.IdleTimeout != idleTimeout {
		t.Fatalf("server timeouts = (%s, %s, %s, %s)", server.ReadHeaderTimeout,
			server.ReadTimeout, server.WriteTimeout, server.IdleTimeout)
	}
}
