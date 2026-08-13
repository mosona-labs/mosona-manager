package notification

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"mosona-manager/internal/_type"
)

func stubNetwork(t *testing.T, addresses ...string) {
	t.Helper()
	oldLookup := lookupIPAddrs
	oldInterfaces := localInterfaces
	lookupIPAddrs = func(context.Context, string) ([]net.IPAddr, error) {
		result := make([]net.IPAddr, 0, len(addresses))
		for _, address := range addresses {
			result = append(result, net.IPAddr{IP: net.ParseIP(address)})
		}
		return result, nil
	}
	localInterfaces = func() ([]net.Addr, error) { return nil, nil }
	t.Cleanup(func() {
		lookupIPAddrs = oldLookup
		localInterfaces = oldInterfaces
	})
}

func TestValidateTargetAcceptsPublicHTTPSGeneric(t *testing.T) {
	stubNetwork(t, "93.184.216.34")
	if err := ValidateTarget(context.Background(), "generic+https://hooks.example.com/notify?template=json"); err != nil {
		t.Fatal(err)
	}
	config, err := parseGenericTarget("generic+https://hooks.example.com/notify?disabletls=yes")
	if err != nil {
		t.Fatal(err)
	}
	if config.webhookURL.Scheme != "https" || config.service.Config.DisableTLS {
		t.Fatalf("custom HTTPS target normalized to %#v", config.webhookURL)
	}
	if err := ValidateTarget(context.Background(), "generic://hooks.example.com/notify"); err != nil {
		t.Fatal(err)
	}
}

func TestValidateTargetRejectsUnsafeGenericTargets(t *testing.T) {
	tests := []string{
		"generic+http://example.com/hook",
		"generic+https://user:pass@example.com/hook",
		"generic://example.com/hook?disabletls=yes",
		"generic://example.com/hook?method=GET",
		"generic://127.0.0.1/hook",
		"generic://[::1]/hook",
		"generic://[fec0::1]/hook",
		"generic://[fe80::1%25en0]/hook",
		"generic://169.254.169.254/latest/meta-data",
	}
	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			if err := ValidateTarget(context.Background(), target); !errors.Is(err, ErrInvalidConfiguration) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestValidateTargetRejectsMixedPublicAndPrivateDNS(t *testing.T) {
	stubNetwork(t, "93.184.216.34", "10.0.0.1")
	err := ValidateTarget(context.Background(), "generic+https://rebind.example/hook")
	if !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("error = %v", err)
	}
}

func TestDialRejectsDNSRebindingAfterValidation(t *testing.T) {
	oldLookup := lookupIPAddrs
	oldInterfaces := localInterfaces
	calls := 0
	lookupIPAddrs = func(context.Context, string) ([]net.IPAddr, error) {
		calls++
		if calls == 1 {
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		}
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}
	localInterfaces = func() ([]net.Addr, error) { return nil, nil }
	t.Cleanup(func() {
		lookupIPAddrs = oldLookup
		localInterfaces = oldInterfaces
	})

	if err := ValidateTarget(context.Background(), "generic+https://rebind.example/hook"); err != nil {
		t.Fatalf("initial validation failed: %v", err)
	}
	if _, err := dialPublicContext(context.Background(), "tcp", "rebind.example:443"); err == nil {
		t.Fatal("dial accepted rebound private address")
	}
}

func TestValidateTargetRejectsLocalInterfaceAddress(t *testing.T) {
	oldLookup := lookupIPAddrs
	oldInterfaces := localInterfaces
	lookupIPAddrs = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}
	localInterfaces = func() ([]net.Addr, error) {
		_, network, _ := net.ParseCIDR("93.184.216.34/32")
		return []net.Addr{network}, nil
	}
	t.Cleanup(func() {
		lookupIPAddrs = oldLookup
		localInterfaces = oldInterfaces
	})

	err := ValidateTarget(context.Background(), "generic+https://local-interface.example/hook")
	if !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateTargetRejectsUnsupportedSchemes(t *testing.T) {
	for _, target := range []string{
		"smtp://user:password@example.com:25/",
		"matrix://user:password@example.com/room",
		"file:///etc/passwd",
	} {
		if err := ValidateTarget(context.Background(), target); !errors.Is(err, ErrInvalidConfiguration) {
			t.Fatalf("target %q: error = %v", target, err)
		}
	}
}

func TestValidateTargetRejectsSlackAPIToken(t *testing.T) {
	target := "slack://bot@xoxb-AAAAAAAAA-BBBBBBBBB-123456789123456789123456"
	if err := ValidateTarget(context.Background(), target); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("error = %v", err)
	}
}

func TestGenericHeadersRejectsHopByHopAndNewlines(t *testing.T) {
	for _, target := range []string{
		"generic+https://example.com/hook?%40Host=internal.example",
		"generic+https://example.com/hook?%40Connection=keep-alive",
		"generic+https://example.com/hook?%40X-Test=ok%0d%0aInjected%3Ayes",
	} {
		if _, err := parseGenericTarget(target); !errors.Is(err, ErrInvalidConfiguration) {
			t.Fatalf("target %q: error = %v", target, err)
		}
	}
}

func TestGenericRedirectPolicy(t *testing.T) {
	client := newGenericHTTPClient()
	viaURL, _ := url.Parse("https://hooks.example.com/start")
	via := []*http.Request{{URL: viaURL}}

	privateURL, _ := url.Parse("https://127.0.0.1/internal")
	if err := client.CheckRedirect(&http.Request{Method: http.MethodPost, URL: privateURL}, via); err == nil {
		t.Fatal("private redirect was accepted")
	}
	crossHostURL, _ := url.Parse("https://other.example.com/hook")
	if err := client.CheckRedirect(&http.Request{Method: http.MethodPost, URL: crossHostURL}, via); err == nil {
		t.Fatal("cross-host redirect was accepted")
	}
	validURL, _ := url.Parse("https://hooks.example.com/next")
	if err := client.CheckRedirect(&http.Request{Method: http.MethodPost, URL: validURL}, via); err != nil {
		t.Fatalf("same-host public redirect rejected: %v", err)
	}
	if err := client.CheckRedirect(&http.Request{Method: http.MethodGet, URL: validURL}, via); err == nil {
		t.Fatal("redirect that changed POST to GET was accepted")
	}
	tooMany := make([]*http.Request, maxGenericRedirects)
	for index := range tooMany {
		tooMany[index] = &http.Request{URL: viaURL}
	}
	if err := client.CheckRedirect(&http.Request{Method: http.MethodPost, URL: validURL}, tooMany); err == nil {
		t.Fatal("redirect limit was not enforced")
	}
}

func TestNormalizeEntriesRejectsExcessiveTargets(t *testing.T) {
	entries := make([]_type.TeamNotification, maxNotificationCount+1)
	_, err := NormalizeEntries(context.Background(), entries)
	if !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("error = %v", err)
	}
}

func TestTargetRateLimiter(t *testing.T) {
	limiter := newTargetRateLimiter(2, time.Minute)
	now := time.Now()
	if !limiter.Allow("target", now) || !limiter.Allow("target", now) {
		t.Fatal("initial calls were rejected")
	}
	if limiter.Allow("target", now) {
		t.Fatal("rate limit was not enforced")
	}
	if !limiter.Allow("target", now.Add(time.Minute)) {
		t.Fatal("rate limit window did not reset")
	}
}

func TestGenericHTTPClientHasBoundedTimeouts(t *testing.T) {
	client := newGenericHTTPClient()
	if client.Timeout != 12*time.Second {
		t.Fatalf("client timeout = %v", client.Timeout)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T", client.Transport)
	}
	if transport.DialContext == nil || transport.TLSHandshakeTimeout == 0 || transport.ResponseHeaderTimeout == 0 {
		t.Fatalf("transport timeouts are incomplete: %#v", transport)
	}
}

func TestHostedHTTPClientRejectsRedirects(t *testing.T) {
	request, _ := http.NewRequest(http.MethodPost, "https://hooks.example.com/next", nil)
	if err := hostedHTTPClient.CheckRedirect(request, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("redirect error = %v", err)
	}
}

func TestGenericResponseReadLimit(t *testing.T) {
	if err := discardBoundedResponse(strings.NewReader(strings.Repeat("x", maxGenericResponseBytes))); err != nil {
		t.Fatalf("bounded response rejected: %v", err)
	}
	if err := discardBoundedResponse(strings.NewReader(strings.Repeat("x", maxGenericResponseBytes+1))); err == nil {
		t.Fatal("oversized response was accepted")
	}
}

func TestNormalizeEntryTrimsAndValidatesEmail(t *testing.T) {
	entry, err := NormalizeEntry(context.Background(), _type.TeamNotification{
		Module: " EMAIL ", Target: " admin@example.com ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if entry.Module != "email" || entry.Target != "admin@example.com" {
		t.Fatalf("entry = %#v", entry)
	}
	_, err = NormalizeEntry(context.Background(), _type.TeamNotification{Module: "email", Target: "Display <admin@example.com>"})
	if !errors.Is(err, ErrInvalidConfiguration) || !strings.Contains(err.Error(), "invalid email") {
		t.Fatalf("error = %v", err)
	}
}
