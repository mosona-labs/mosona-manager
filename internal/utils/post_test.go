package utils

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPostFormContextCancelsRequest(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- PostFormContext(ctx, server.URL, nil, nil, nil)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("request did not reach the server")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("PostFormContext() error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("PostFormContext did not stop after cancellation")
	}
}
