package utils

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestVerifyCaptchaUsesRequestContext(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := verifyCaptcha(ctx, client, captchaVerifyURL, "secret", "token", "127.0.0.1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestVerifyCaptchaRejectsOversizedResponse(t *testing.T) {
	response := `{"success":true,"padding":"` + strings.Repeat("x", maxCaptchaResponseBytes) + `"}`
	client := captchaTestClient(http.StatusOK, response)

	_, err := verifyCaptcha(context.Background(), client, captchaVerifyURL, "secret", "token", "127.0.0.1")
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v, want response size error", err)
	}
}

func TestVerifyCaptchaRejectsErrorStatus(t *testing.T) {
	client := captchaTestClient(http.StatusBadGateway, `{"success":false}`)
	_, err := verifyCaptcha(context.Background(), client, captchaVerifyURL, "secret", "token", "127.0.0.1")
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("error = %v, want status error", err)
	}
}

func captchaTestClient(status int, body string) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})}
}
