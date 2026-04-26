package app

import (
	"net/http"
	"testing"
)

func TestCDNClientIPExtractor(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string][]string
		remoteAddr string
		want       string
	}{
		{
			name:       "cloudflare header wins",
			headers:    map[string][]string{"CF-Connecting-IP": {"203.0.113.10"}},
			remoteAddr: "10.0.0.8:443",
			want:       "203.0.113.10",
		},
		{
			name:       "x-forwarded-for uses original client",
			headers:    map[string][]string{"X-Forwarded-For": {"198.51.100.7, 10.0.0.8"}},
			remoteAddr: "10.0.0.8:443",
			want:       "198.51.100.7",
		},
		{
			name:       "skips invalid header values",
			headers:    map[string][]string{"CF-Connecting-IP": {"unknown"}, "X-Real-IP": {"198.51.100.11"}},
			remoteAddr: "10.0.0.8:443",
			want:       "198.51.100.11",
		},
		{
			name:       "falls back to remote address",
			headers:    map[string][]string{},
			remoteAddr: "10.0.0.8:443",
			want:       "10.0.0.8",
		},
		{
			name:       "accepts ipv6",
			headers:    map[string][]string{"X-Forwarded-For": {"2001:db8::1, 10.0.0.8"}},
			remoteAddr: "10.0.0.8:443",
			want:       "2001:db8::1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{Header: http.Header{}, RemoteAddr: tt.remoteAddr}
			for key, values := range tt.headers {
				for _, value := range values {
					req.Header.Add(key, value)
				}
			}
			if got := cdnClientIPExtractor(req); got != tt.want {
				t.Fatalf("cdnClientIPExtractor() = %q, want %q", got, tt.want)
			}
		})
	}
}
