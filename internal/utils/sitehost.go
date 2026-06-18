package utils

import (
	"net"
	"net/url"
	"strings"
)

func SiteHostFromBaseURL(raw string) string {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return ""
	}
	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		if err != nil || parsed.Host == "" {
			return ""
		}
		value = parsed.Host
	}
	host, _, err := net.SplitHostPort(value)
	if err == nil {
		value = host
	}
	return strings.TrimSpace(strings.TrimSuffix(value, "."))
}

func NormalizeRequestHost(host string) string {
	value := strings.TrimSpace(strings.ToLower(host))
	if value == "" {
		return ""
	}
	h, _, err := net.SplitHostPort(value)
	if err == nil {
		value = h
	}
	return strings.TrimSpace(strings.TrimSuffix(value, "."))
}

func RequestHostMatchesBaseURL(requestHost, baseURL string) bool {
	base := SiteHostFromBaseURL(baseURL)
	if base == "" {
		return true
	}
	req := NormalizeRequestHost(requestHost)
	return req != "" && strings.EqualFold(req, base)
}
