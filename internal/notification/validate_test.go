package notification

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"mosona-manager/internal/_type"
)

func TestValidateTargetAcceptsHTTPAndHTTPSGeneric(t *testing.T) {
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
	for _, target := range []string{
		"generic+http://127.0.0.1:8080/hook",
		"generic://10.0.0.2/hook?disabletls=yes",
		"generic+http://[::1]:8080/hook",
	} {
		if err := ValidateTarget(context.Background(), target); err != nil {
			t.Fatalf("target %q: %v", target, err)
		}
	}
}

func TestValidateTargetRejectsMalformedGenericTargets(t *testing.T) {
	tests := []string{
		"generic+https://user:pass@example.com/hook",
		"generic://example.com/hook?method=GET",
		"generic+ftp://example.com/hook",
		"generic+http:///hook",
	}
	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			if err := ValidateTarget(context.Background(), target); !errors.Is(err, ErrInvalidConfiguration) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestValidateTargetAcceptsSelfHostedServices(t *testing.T) {
	for _, target := range []string{
		"gotify://10.0.0.2/Aaa.bbb.ccc.ddd?disabletls=yes",
		"ntfy://192.168.1.20/alerts?scheme=http",
		"bark://:device-key@172.16.0.8?scheme=http",
		"smtp://user:password@mail.internal:25/?fromAddress=sender@example.com&toAddresses=ops@example.com&useStartTLS=no",
	} {
		if err := ValidateTarget(context.Background(), target); err != nil {
			t.Fatalf("target %q: error = %v", target, err)
		}
	}
}

func TestValidateTargetRejectsUnsupportedAndMalformedServices(t *testing.T) {
	for _, target := range []string{
		"unknown://example.com/path",
		"file:///etc/passwd",
		"ntfy://example.com",
	} {
		if err := ValidateTarget(context.Background(), target); !errors.Is(err, ErrInvalidConfiguration) {
			t.Fatalf("target %q: error = %v", target, err)
		}
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

	crossHostURL, _ := url.Parse("https://other.example.com/hook")
	if err := client.CheckRedirect(&http.Request{Method: http.MethodPost, URL: crossHostURL}, via); err == nil {
		t.Fatal("cross-host redirect was accepted")
	}
	downgradeURL, _ := url.Parse("http://hooks.example.com/hook")
	if err := client.CheckRedirect(&http.Request{Method: http.MethodPost, URL: downgradeURL}, via); err == nil {
		t.Fatal("HTTPS-to-HTTP redirect was accepted")
	}
	validURL, _ := url.Parse("https://hooks.example.com/next")
	if err := client.CheckRedirect(&http.Request{Method: http.MethodPost, URL: validURL}, via); err != nil {
		t.Fatalf("same-host public redirect rejected: %v", err)
	}
	privateHTTPURL, _ := url.Parse("http://127.0.0.1/internal")
	privateViaURL, _ := url.Parse("http://127.0.0.1/start")
	if err := client.CheckRedirect(&http.Request{Method: http.MethodPost, URL: privateHTTPURL}, []*http.Request{{URL: privateViaURL}}); err != nil {
		t.Fatalf("same-host private HTTP redirect rejected: %v", err)
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

func TestSendGenericDeliversToPrivateHTTPWebhook(t *testing.T) {
	received := make(chan string, 1)
	oldDial := genericDialContext
	genericDialContext = func(context.Context, string, string) (net.Conn, error) {
		client, server := net.Pipe()
		go func() {
			defer func() { _ = server.Close() }()
			request, err := http.ReadRequest(bufio.NewReader(server))
			if err != nil {
				received <- "read error: " + err.Error()
				return
			}
			body, _ := io.ReadAll(request.Body)
			received <- string(body)
			_, _ = io.WriteString(server, "HTTP/1.1 204 No Content\r\nContent-Length: 0\r\n\r\n")
		}()
		return client, nil
	}
	t.Cleanup(func() { genericDialContext = oldDial })

	target := "generic+http://127.0.0.1:8080/hook?template=json"
	if err := Send(context.Background(), target, "private webhook test"); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-received:
		if !strings.Contains(body, "private webhook test") {
			t.Fatalf("body = %q", body)
		}
	case <-time.After(time.Second):
		t.Fatal("private webhook did not receive notification")
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
