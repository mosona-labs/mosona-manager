package notification

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	"github.com/nicholas-fedor/shoutrrr/pkg/services/specialized/generic"
	"github.com/nicholas-fedor/shoutrrr/pkg/types"
)

const (
	maxGenericResponseBytes = 64 << 10
	maxGenericRedirects     = 3
)

var (
	dialer             = &net.Dialer{Timeout: 3 * time.Second, KeepAlive: 15 * time.Second}
	genericDialContext = dialer.DialContext
)

type genericTarget struct {
	service    *generic.Service
	webhookURL *url.URL
	headers    http.Header
}

func parseGenericTarget(raw string) (*genericTarget, error) {
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Opaque != "" {
		return nil, fmt.Errorf("%w: malformed generic webhook", ErrInvalidConfiguration)
	}

	service := &generic.Service{}
	serviceURL := parsed
	if strings.EqualFold(parsed.Scheme, "generic+https") || strings.EqualFold(parsed.Scheme, "generic+http") {
		serviceURL, err = service.GetServiceURLFromCustom(parsed)
		if err != nil {
			return nil, fmt.Errorf("%w: malformed generic webhook", ErrInvalidConfiguration)
		}
	} else if !strings.EqualFold(parsed.Scheme, "generic") {
		return nil, fmt.Errorf("%w: generic webhook must use HTTP or HTTPS", ErrInvalidConfiguration)
	}

	if err := service.Initialize(serviceURL, nil); err != nil || service.Config == nil {
		return nil, fmt.Errorf("%w: malformed generic webhook", ErrInvalidConfiguration)
	}
	webhookURL := service.Config.WebhookURL()
	if (webhookURL.Scheme != "http" && webhookURL.Scheme != "https") || webhookURL.Hostname() == "" || webhookURL.Fragment != "" || webhookURL.User != nil {
		return nil, fmt.Errorf("%w: generic webhook must use HTTP or HTTPS", ErrInvalidConfiguration)
	}
	if strings.ToUpper(service.Config.RequestMethod) != http.MethodPost {
		return nil, fmt.Errorf("%w: generic webhook only supports POST", ErrInvalidConfiguration)
	}
	switch service.Config.Template {
	case "", "json", generic.JSONTemplate:
	default:
		return nil, fmt.Errorf("%w: generic webhook template is not allowed", ErrInvalidConfiguration)
	}
	if strings.ContainsAny(service.Config.ContentType, "\r\n") || len(service.Config.ContentType) > 256 {
		return nil, fmt.Errorf("%w: invalid generic webhook content type", ErrInvalidConfiguration)
	}

	headers, err := genericHeaders(parsed.Query())
	if err != nil {
		return nil, err
	}
	return &genericTarget{service: service, webhookURL: webhookURL, headers: headers}, nil
}

func genericHeaders(query url.Values) (http.Header, error) {
	headers := make(http.Header)
	total := 0
	for key, values := range query {
		if !strings.HasPrefix(key, "@") || len(key) == 1 || len(values) == 0 {
			continue
		}
		name := textproto.CanonicalMIMEHeaderKey(key[1:])
		value := values[0]
		if name == "" || strings.ContainsAny(name+value, "\r\n") {
			return nil, fmt.Errorf("%w: invalid generic webhook header", ErrInvalidConfiguration)
		}
		switch name {
		case "Host", "Content-Length", "Transfer-Encoding", "Connection", "Proxy-Authorization", "Trailer", "Upgrade":
			return nil, fmt.Errorf("%w: generic webhook header is not allowed", ErrInvalidConfiguration)
		}
		total += len(name) + len(value)
		if total > 8192 {
			return nil, fmt.Errorf("%w: generic webhook headers are too large", ErrInvalidConfiguration)
		}
		headers.Set(name, value)
	}
	return headers, nil
}

func sendGeneric(ctx context.Context, target, message string) error {
	config, err := parseGenericTarget(target)
	if err != nil {
		return err
	}
	params := types.Params{config.service.Config.MessageKey: message}
	payload, err := config.service.GetPayload(config.service.Config, params)
	if err != nil {
		return fmt.Errorf("prepare generic webhook payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.webhookURL.String(), payload)
	if err != nil {
		return fmt.Errorf("create generic webhook request: %w", err)
	}
	contentType := config.service.Config.ContentType
	if config.service.Config.Template == "" {
		contentType = "text/plain"
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", contentType)
	for key, values := range config.headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	client := newGenericHTTPClient()
	defer client.CloseIdleConnections()
	response, err := client.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return context.DeadlineExceeded
		}
		return errors.New("generic webhook request failed")
	}
	defer func() { _ = response.Body.Close() }()

	if err := discardBoundedResponse(response.Body); err != nil {
		return err
	}
	if response.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("generic webhook returned HTTP status %d", response.StatusCode)
	}
	return nil
}

func discardBoundedResponse(body io.Reader) error {
	read, err := io.Copy(io.Discard, io.LimitReader(body, maxGenericResponseBytes+1))
	if err != nil {
		return errors.New("generic webhook response could not be read")
	}
	if read > maxGenericResponseBytes {
		return errors.New("generic webhook response is too large")
	}
	return nil
}

func newGenericHTTPClient() *http.Client {
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           genericDialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          8,
		MaxIdleConnsPerHost:   2,
		IdleConnTimeout:       15 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12}, //nolint:gosec // TLS 1.2 is the compatibility floor.
		ResponseHeaderTimeout: 5 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   12 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxGenericRedirects {
				return errors.New("generic webhook redirect limit exceeded")
			}
			if (req.URL.Scheme != "http" && req.URL.Scheme != "https") || req.URL.Hostname() == "" {
				return errors.New("generic webhook redirected to an invalid URL")
			}
			if req.Method != http.MethodPost {
				return errors.New("generic webhook redirect changed the request method")
			}
			if len(via) > 0 && (!strings.EqualFold(req.URL.Hostname(), via[0].URL.Hostname()) || !strings.EqualFold(req.URL.Scheme, via[0].URL.Scheme)) {
				return errors.New("generic webhook cross-origin redirect is not allowed")
			}
			return nil
		},
	}
}
