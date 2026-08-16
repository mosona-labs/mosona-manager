package passive

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"testing"
)

func TestReconnectAuthCallbackOnlyRefreshesAuthentication(t *testing.T) {
	calls := 0
	callback := reconnectAuthCallback(func() error {
		calls++
		return nil
	})

	for range 100 {
		callback()
	}

	if calls != 100 {
		t.Fatalf("authentication refresh calls = %d, want 100", calls)
	}
}

func TestReconnectAuthCallbackLogsRefreshError(t *testing.T) {
	var logOutput bytes.Buffer
	oldWriter := log.Writer()
	log.SetOutput(&logOutput)
	t.Cleanup(func() { log.SetOutput(oldWriter) })

	calls := 0
	callback := reconnectAuthCallback(func() error {
		calls++
		return errors.New("token refresh failed")
	})

	callback()

	if calls != 1 {
		t.Fatalf("authentication refresh calls = %d, want 1", calls)
	}
	if !strings.Contains(logOutput.String(), "token refresh failed") {
		t.Fatalf("expected the refresh error to be logged, got %q", logOutput.String())
	}
}
