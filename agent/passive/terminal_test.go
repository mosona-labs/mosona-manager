package passive

import (
	"errors"
	"net/http"
	"testing"

	"mosona-manager/pkg/ws"
)

type terminalConnectCall struct {
	path      string
	sessionID string
}

func TestConnectTerminalHubUsesHeaderEndpointWhenAvailable(t *testing.T) {
	wantClient := ws.NewClient()
	t.Cleanup(func() { _ = wantClient.Close() })
	var calls []terminalConnectCall

	client, err := connectTerminalHubWith("session-id", func(path, sessionID string) (*ws.Client, error) {
		calls = append(calls, terminalConnectCall{path: path, sessionID: sessionID})
		return wantClient, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if client != wantClient {
		t.Fatal("connector returned an unexpected client")
	}
	assertTerminalConnectCalls(t, calls, []terminalConnectCall{
		{path: "/api/agent/terminal", sessionID: "session-id"},
	})
}

func TestConnectTerminalHubFallsBackForLegacyHub(t *testing.T) {
	const sessionID = "550e8400-e29b-41d4-a716-446655440000"
	for _, statusCode := range []int{http.StatusNotFound, http.StatusMethodNotAllowed} {
		t.Run(http.StatusText(statusCode), func(t *testing.T) {
			wantClient := ws.NewClient()
			t.Cleanup(func() { _ = wantClient.Close() })
			var calls []terminalConnectCall

			client, err := connectTerminalHubWith(sessionID, func(path, sessionID string) (*ws.Client, error) {
				calls = append(calls, terminalConnectCall{path: path, sessionID: sessionID})
				if len(calls) == 1 {
					return nil, &ws.HandshakeError{
						StatusCode: statusCode,
						Status:     http.StatusText(statusCode),
						Err:        errors.New("bad handshake"),
					}
				}
				return wantClient, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if client != wantClient {
				t.Fatal("legacy connector returned an unexpected client")
			}
			assertTerminalConnectCalls(t, calls, []terminalConnectCall{
				{path: "/api/agent/terminal", sessionID: sessionID},
				{path: "/api/agent/terminal/" + sessionID, sessionID: sessionID},
			})
		})
	}
}

func TestConnectTerminalHubDoesNotFallbackForOtherFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "unauthorized",
			err: &ws.HandshakeError{
				StatusCode: http.StatusUnauthorized,
				Status:     http.StatusText(http.StatusUnauthorized),
				Err:        errors.New("bad handshake"),
			},
		},
		{
			name: "forbidden",
			err: &ws.HandshakeError{
				StatusCode: http.StatusForbidden,
				Status:     http.StatusText(http.StatusForbidden),
				Err:        errors.New("bad handshake"),
			},
		},
		{
			name: "server error",
			err: &ws.HandshakeError{
				StatusCode: http.StatusInternalServerError,
				Status:     http.StatusText(http.StatusInternalServerError),
				Err:        errors.New("bad handshake"),
			},
		},
		{name: "network error", err: errors.New("connection refused")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			_, err := connectTerminalHubWith("session-id", func(string, string) (*ws.Client, error) {
				calls++
				return nil, test.err
			})
			if !errors.Is(err, test.err) {
				t.Fatalf("error = %v, want %v", err, test.err)
			}
			if calls != 1 {
				t.Fatalf("connect calls = %d, want 1", calls)
			}
		})
	}
}

func assertTerminalConnectCalls(t *testing.T, got, want []terminalConnectCall) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("connect calls = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("connect call %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
