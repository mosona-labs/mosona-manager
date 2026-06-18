package httporigin

import (
	"net/http/httptest"
	"testing"
)

func TestSameOrigin(t *testing.T) {
	tests := []struct {
		name   string
		host   string
		origin string
		want   bool
	}{
		{
			name: "missing origin is allowed for non browser clients",
			host: "hub.example.com",
			want: true,
		},
		{
			name:   "matching origin host is allowed",
			host:   "hub.example.com",
			origin: "https://hub.example.com",
			want:   true,
		},
		{
			name:   "matching origin host is case insensitive",
			host:   "hub.example.com",
			origin: "https://HUB.EXAMPLE.COM",
			want:   true,
		},
		{
			name:   "different origin host is rejected",
			host:   "hub.example.com",
			origin: "https://evil.example.com",
			want:   false,
		},
		{
			name:   "invalid origin is rejected",
			host:   "hub.example.com",
			origin: "://bad",
			want:   false,
		},
		{
			name:   "origin without host is rejected",
			host:   "hub.example.com",
			origin: "/relative",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://"+tt.host+"/ws", nil)
			req.Host = tt.host
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}

			if got := SameOrigin(req); got != tt.want {
				t.Fatalf("SameOrigin() = %v, want %v", got, tt.want)
			}
		})
	}
}
