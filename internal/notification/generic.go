package notification

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
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
	lookupIPAddrs   = net.DefaultResolver.LookupIPAddr
	localInterfaces = net.InterfaceAddrs
	dialer          = &net.Dialer{Timeout: 3 * time.Second, KeepAlive: 15 * time.Second}
	lookupSlots     = make(chan struct{}, 8)

	blockedPrefixes = mustPrefixes(
		"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8",
		"169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24",
		"192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24",
		"224.0.0.0/4", "240.0.0.0/4", "::/96", "100::/64",
		"64:ff9b::/96", "64:ff9b:1::/48", "2001::/23", "2001:db8::/32", "2002::/16",
		"fc00::/7", "fec0::/10", "fe80::/10", "ff00::/8",
	)
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
	if strings.EqualFold(parsed.Scheme, "generic+https") {
		serviceURL, err = service.GetServiceURLFromCustom(parsed)
		if err != nil {
			return nil, fmt.Errorf("%w: malformed generic webhook", ErrInvalidConfiguration)
		}
	} else if !strings.EqualFold(parsed.Scheme, "generic") {
		return nil, fmt.Errorf("%w: generic webhook must use HTTPS", ErrInvalidConfiguration)
	}

	if err := service.Initialize(serviceURL, nil); err != nil || service.Config == nil {
		return nil, fmt.Errorf("%w: malformed generic webhook", ErrInvalidConfiguration)
	}
	webhookURL := service.Config.WebhookURL()
	if webhookURL.Scheme != "https" || webhookURL.Hostname() == "" || webhookURL.Fragment != "" || webhookURL.User != nil {
		return nil, fmt.Errorf("%w: generic webhook must use HTTPS", ErrInvalidConfiguration)
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
	if err := validatePublicDestination(ctx, config.webhookURL, true); err != nil {
		return fmt.Errorf("%w: generic webhook destination is not allowed", ErrInvalidConfiguration)
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
		DialContext:           dialPublicContext,
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
			if req.URL.Scheme != "https" || req.URL.Hostname() == "" {
				return errors.New("generic webhook redirected to an invalid URL")
			}
			if req.Method != http.MethodPost {
				return errors.New("generic webhook redirect changed the request method")
			}
			if len(via) > 0 && !strings.EqualFold(req.URL.Hostname(), via[0].URL.Hostname()) {
				return errors.New("generic webhook cross-host redirect is not allowed")
			}
			return validatePublicDestination(req.Context(), req.URL, false)
		},
	}
}

func dialPublicContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid webhook address: %w", err)
	}
	addresses, err := resolvePublicAddresses(ctx, host)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, ip := range addresses {
		conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		lastErr = dialErr
	}
	if lastErr == nil {
		lastErr = errors.New("webhook host has no allowed addresses")
	}
	return nil, lastErr
}

func validatePublicDestination(ctx context.Context, target *url.URL, resolve bool) error {
	if target == nil || target.Scheme != "https" || target.Hostname() == "" {
		return errors.New("destination must be an HTTPS URL")
	}
	if ip, err := netip.ParseAddr(strings.Trim(target.Hostname(), "[]")); err == nil {
		if !isPublicAddress(ip) || isLocalInterface(ip) {
			return errors.New("destination address is not public")
		}
		return nil
	}
	if !resolve {
		return nil
	}
	_, err := resolvePublicAddresses(ctx, target.Hostname())
	return err
}

func resolvePublicAddresses(ctx context.Context, host string) ([]netip.Addr, error) {
	if parsed, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		if !isPublicAddress(parsed) || isLocalInterface(parsed) {
			return nil, errors.New("destination address is not public")
		}
		return []netip.Addr{parsed.Unmap()}, nil
	}

	select {
	case lookupSlots <- struct{}{}:
		defer func() { <-lookupSlots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	resolved, err := lookupIPAddrs(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve webhook host: %w", err)
	}
	addresses := make([]netip.Addr, 0, len(resolved))
	for _, item := range resolved {
		ip, ok := netip.AddrFromSlice(item.IP)
		if !ok || !isPublicAddress(ip) || isLocalInterface(ip) {
			return nil, errors.New("webhook host resolves to a non-public address")
		}
		addresses = append(addresses, ip.Unmap())
	}
	if len(addresses) == 0 {
		return nil, errors.New("webhook host has no addresses")
	}
	return addresses, nil
}

func isPublicAddress(ip netip.Addr) bool {
	if ip.Zone() != "" {
		return false
	}
	ip = ip.Unmap()
	if !ip.IsValid() || !ip.IsGlobalUnicast() {
		return false
	}
	for _, prefix := range blockedPrefixes {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

func isLocalInterface(ip netip.Addr) bool {
	addresses, err := localInterfaces()
	if err != nil {
		return true
	}
	ip = ip.Unmap()
	for _, address := range addresses {
		var raw string
		switch value := address.(type) {
		case *net.IPNet:
			raw = value.IP.String()
		case *net.IPAddr:
			raw = value.IP.String()
		default:
			continue
		}
		local, err := netip.ParseAddr(raw)
		if err == nil && local.Unmap() == ip {
			return true
		}
	}
	return false
}

func mustPrefixes(values ...string) []netip.Prefix {
	prefixes := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		prefixes = append(prefixes, netip.MustParsePrefix(value))
	}
	return prefixes
}
