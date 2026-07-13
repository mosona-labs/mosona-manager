package httpclient

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func responseClient(status int, body string) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})}
}

func TestPostFormRejectsNonSuccessStatus(t *testing.T) {
	err := postForm(responseClient(http.StatusUnauthorized, `{"code":"unauthorized"}`), "http://agent.test", map[string]interface{}{}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "status 401") {
		t.Fatalf("expected HTTP status error, got %v", err)
	}
}

func TestPostFormDecodesSuccessResponse(t *testing.T) {
	var response struct {
		Code string `json:"code"`
	}
	if err := postForm(responseClient(http.StatusOK, `{"code":"ok"}`), "http://agent.test", map[string]interface{}{}, nil, &response); err != nil {
		t.Fatal(err)
	}
	if response.Code != "ok" {
		t.Fatalf("unexpected response: %#v", response)
	}
}
