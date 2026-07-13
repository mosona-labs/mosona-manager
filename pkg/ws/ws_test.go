package ws

import (
	"testing"
	"time"
)

func TestReconnectDelayBackoffAndCap(t *testing.T) {
	client := NewClient()
	client.SetReconnectBackoff(-1, 5*time.Second, time.Minute)

	tests := []struct {
		retries int
		base    time.Duration
	}{
		{retries: 0, base: 5 * time.Second},
		{retries: 1, base: 10 * time.Second},
		{retries: 4, base: time.Minute},
		{retries: 10, base: time.Minute},
	}
	for _, tt := range tests {
		delay := client.reconnectDelay(tt.retries)
		min := tt.base - tt.base/5
		max := tt.base + tt.base/5
		if delay < min || delay > max {
			t.Fatalf("retries=%d delay=%v outside [%v,%v]", tt.retries, delay, min, max)
		}
	}
}
