package app

import (
	"net"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
)

var cdnClientIPHeaders = []string{
	"CF-Connecting-IPv6",
	"CF-Connecting-IP",
	"True-Client-IP",
	"Fastly-Client-IP",
	"Fly-Client-IP",
	"X-Forwarded-For",
	"X-Real-IP",
	"X-Client-IP",
	"X-Cluster-Client-IP",
}

func configureClientIPExtractor(e *echo.Echo) {
	e.IPExtractor = cdnClientIPExtractor
}

func cdnClientIPExtractor(req *http.Request) string {
	for _, header := range cdnClientIPHeaders {
		for _, value := range req.Header.Values(header) {
			if ip := firstValidIP(value); ip != "" {
				return ip
			}
		}
	}
	return directRemoteIP(req.RemoteAddr)
}

func firstValidIP(value string) string {
	for _, part := range strings.Split(value, ",") {
		ip := normalizeIP(part)
		if ip != "" {
			return ip
		}
	}
	return ""
}

func normalizeIP(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "[]")
	if value == "" || strings.EqualFold(value, "unknown") {
		return ""
	}

	if ip := net.ParseIP(value); ip != nil {
		return ip.String()
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		host = strings.Trim(host, "[]")
		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
	}
	return ""
}

func directRemoteIP(remoteAddr string) string {
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return normalizeIP(host)
	}
	return normalizeIP(remoteAddr)
}
