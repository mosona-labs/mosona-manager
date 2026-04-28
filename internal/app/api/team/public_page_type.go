package ateam

import (
	"errors"
	"mosona-manager/internal/config"
	"net"
	"net/url"
	"strings"

	"github.com/labstack/echo/v5"
)

type publicPageUpdateRequest struct {
	Enabled     bool   `json:"enabled"`
	Name        string `json:"name"`
	Domain      string `json:"domain"`
	Title       string `json:"title"`
	Description string `json:"description"`
	CustomCSS   string `json:"custom_css"`
}

type publicPageResponse struct {
	Enabled     bool    `json:"enabled"`
	Name        *string `json:"name,omitempty"`
	Domain      *string `json:"domain,omitempty"`
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	CustomCSS   *string `json:"custom_css,omitempty"`
	URLByName   *string `json:"url_by_name,omitempty"`
	URLByDomain *string `json:"url_by_domain,omitempty"`
}

func normalizePublicPageName(raw string) (*string, error) {
	name := strings.ToLower(strings.TrimSpace(raw))
	if name == "" {
		return nil, nil
	}
	if len(name) < 3 || len(name) > 32 {
		return nil, errors.New("name length must be between 3 and 32")
	}
	for i, ch := range name {
		switch {
		case ch >= 'a' && ch <= 'z':
		case ch >= '0' && ch <= '9':
		case ch == '-':
			if i == 0 || i == len(name)-1 {
				return nil, errors.New("name cannot start or end with hyphen")
			}
		default:
			return nil, errors.New("name can only contain a-z, 0-9, and hyphen")
		}
	}
	return &name, nil
}

func normalizePublicPageDomain(raw string) (*string, error) {
	domain := strings.ToLower(strings.TrimSpace(raw))
	if domain == "" {
		return nil, nil
	}
	if strings.Contains(domain, "://") || strings.ContainsAny(domain, "/?#@") {
		return nil, errors.New("domain must be a host without scheme or path")
	}
	if len(domain) > 255 {
		return nil, errors.New("domain is too long")
	}

	labels := strings.Split(domain, ".")
	for _, label := range labels {
		if label == "" {
			return nil, errors.New("domain contains an empty label")
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return nil, errors.New("domain labels cannot start or end with hyphen")
		}
		for _, ch := range label {
			switch {
			case ch >= 'a' && ch <= 'z':
			case ch >= '0' && ch <= '9':
			case ch == '-':
			default:
				return nil, errors.New("domain can only contain a-z, 0-9, dot, and hyphen")
			}
		}
	}

	return &domain, nil
}

func normalizePublicPageTitle(raw string) (*string, error) {
	title := strings.TrimSpace(raw)
	if title == "" {
		return nil, nil
	}
	if len(title) > 255 {
		return nil, errors.New("title is too long")
	}
	return &title, nil
}

func normalizePublicPageDescription(raw string) *string {
	description := strings.TrimSpace(raw)
	if description == "" {
		return nil
	}
	return &description
}

func normalizePublicPageCustomCSS(raw string) *string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return &raw
}

func normalizeConfiguredBaseDomain(raw string) string {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return ""
	}

	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		if err != nil {
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

func requestScheme(c *echo.Context) string {
	for _, key := range []string{"X-Forwarded-Proto", "X-Forwarded-Scheme"} {
		if value := strings.TrimSpace(c.Request().Header.Get(key)); value != "" {
			return strings.TrimSpace(strings.Split(value, ",")[0])
		}
	}
	if scheme := c.Scheme(); scheme != "" {
		return scheme
	}
	return "http"
}

func publicAppBaseURL(c *echo.Context) string {
	if domain := strings.TrimSpace(config.ReadDynamicConf().Domain); domain != "" {
		return strings.TrimRight(domain, "/")
	}
	return requestScheme(c) + "://" + c.Request().Host
}
