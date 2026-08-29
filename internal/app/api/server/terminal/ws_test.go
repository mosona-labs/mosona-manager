package aterminal

import (
	"context"
	"errors"
	"testing"

	"mosona-manager/internal/connect/active"
)

func TestActiveTerminalErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "canceled", err: context.Canceled, want: ""},
		{name: "wrapped canceled", err: errors.Join(errors.New("closed"), context.Canceled), want: ""},
		{name: "unpaired", err: active.ErrAgentIdentityUnpaired, want: "Active Agent identity verification failed. Reinstall or verify the Agent before opening a terminal.\n"},
		{name: "mismatch", err: active.ErrAgentIdentityMismatch, want: "Active Agent identity verification failed. Reinstall or verify the Agent before opening a terminal.\n"},
		{name: "other", err: errors.New("dial failed"), want: "Unable to establish an authenticated Active Agent terminal.\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := activeTerminalErrorMessage(tt.err); got != tt.want {
				t.Fatalf("activeTerminalErrorMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}
