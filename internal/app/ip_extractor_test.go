package app

import (
	"mosona-manager/internal/config"
	"net/http"
	"testing"
)

func TestCDNClientIPExtractor(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string][]string
		remoteAddr string
		want       string
		trustProxy bool
	}{
		{
			name:       "ignores forwarded headers when trust proxy disabled",
			headers:    map[string][]string{"CF-Connecting-IP": {"203.0.113.10"}},
			remoteAddr: "10.0.0.8:443",
			want:       "10.0.0.8",
		},
		{
			name:       "trust proxy uses cloudflare header",
			headers:    map[string][]string{"CF-Connecting-IP": {"203.0.113.10"}},
			remoteAddr: "10.0.0.8:443",
			want:       "203.0.113.10",
			trustProxy: true,
		},
		{
			name:       "trust proxy x-forwarded-for uses original client",
			headers:    map[string][]string{"X-Forwarded-For": {"198.51.100.7, 10.0.0.8"}},
			remoteAddr: "10.0.0.8:443",
			want:       "198.51.100.7",
			trustProxy: true,
		},
		{
			name:       "falls back to remote address",
			headers:    map[string][]string{},
			remoteAddr: "10.0.0.8:443",
			want:       "10.0.0.8",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old := config.ReadDynamicConf()
			next := old
			next.TrustProxy = tt.trustProxy
			config.ReplaceDynamicConf(next)
			t.Cleanup(func() { config.ReplaceDynamicConf(old) })

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
