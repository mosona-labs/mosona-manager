package httporigin

import (
	"net/http"
	"net/url"
	"strings"
)

func SameOrigin(r *http.Request) bool {
	return SameOriginHeader(r, "Origin")
}

func SameOriginHeader(r *http.Request, header string) bool {
	value := strings.TrimSpace(r.Header.Get(header))
	if value == "" {
		return true
	}

	u, err := url.Parse(value)
	if err != nil || u.Host == "" {
		return false
	}

	return strings.EqualFold(u.Host, r.Host)
}
