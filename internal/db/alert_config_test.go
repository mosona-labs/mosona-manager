package db

import (
	"errors"
	"testing"
)

func TestNormalizeAlertConfigEnforcesDeclaredBounds(t *testing.T) {
	tests := []struct {
		name            string
		item            string
		threshold       int
		forDuration     int
		wantThreshold   int
		wantForDuration int
		wantErr         error
	}{
		{name: "status minimum", item: "status", threshold: 99, forDuration: 1, wantForDuration: 1},
		{name: "status maximum", item: "status", threshold: 99, forDuration: 1440, wantForDuration: 1440},
		{name: "status duration below minimum", item: "status", forDuration: 0, wantErr: ErrAlertInvalidConfig},
		{name: "status duration above maximum", item: "status", forDuration: 1441, wantErr: ErrAlertInvalidConfig},
		{name: "percentage minimum", item: "cpu_usage", threshold: 1, forDuration: 1, wantThreshold: 1, wantForDuration: 1},
		{name: "percentage maximum", item: "memory_usage", threshold: 100, forDuration: 1440, wantThreshold: 100, wantForDuration: 1440},
		{name: "percentage below minimum", item: "disk_usage", threshold: 0, forDuration: 10, wantErr: ErrAlertInvalidConfig},
		{name: "percentage above maximum", item: "cpu_usage", threshold: 101, forDuration: 10, wantErr: ErrAlertInvalidConfig},
		{name: "iops maximum", item: "read_iops", threshold: 1_000_000, forDuration: 10, wantThreshold: 1_000_000, wantForDuration: 10},
		{name: "iops above maximum", item: "write_iops", threshold: 1_000_001, forDuration: 10, wantErr: ErrAlertInvalidConfig},
		{name: "bandwidth minimum", item: "bandwidth", threshold: 1, forDuration: 10, wantThreshold: 1, wantForDuration: 10},
		{name: "ordinary duration below minimum", item: "bandwidth", threshold: 100, forDuration: -1, wantErr: ErrAlertInvalidConfig},
		{name: "ordinary duration above maximum", item: "read_iops", threshold: 1000, forDuration: 1441, wantErr: ErrAlertInvalidConfig},
		{name: "expiry minimum", item: "expiry_reminder", threshold: 1, forDuration: 99, wantThreshold: 1},
		{name: "expiry maximum", item: "expiry_reminder", threshold: 7, forDuration: 99, wantThreshold: 7},
		{name: "expiry below minimum", item: "expiry_reminder", threshold: 0, wantErr: ErrAlertInvalidConfig},
		{name: "expiry above maximum", item: "expiry_reminder", threshold: 8, wantErr: ErrAlertInvalidConfig},
		{name: "unknown item", item: "unknown", threshold: 1, forDuration: 1, wantErr: ErrAlertNotFound},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			threshold, forDuration, err := normalizeAlertConfig(test.item, test.threshold, test.forDuration)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("normalizeAlertConfig() error = %v, want %v", err, test.wantErr)
			}
			if threshold != test.wantThreshold || forDuration != test.wantForDuration {
				t.Fatalf("normalizeAlertConfig() = (%d, %d), want (%d, %d)", threshold, forDuration, test.wantThreshold, test.wantForDuration)
			}
		})
	}
}

func TestAlertRuleFieldBoundsAreWellFormed(t *testing.T) {
	for item, rule := range alertItemRules {
		for name, field := range map[string]struct {
			enabled       bool
			min, max, def int
		}{
			"threshold":    {rule.threshold.Enabled, rule.threshold.Min, rule.threshold.Max, rule.threshold.Default},
			"for_duration": {rule.forDuration.Enabled, rule.forDuration.Min, rule.forDuration.Max, rule.forDuration.Default},
		} {
			if !field.enabled {
				continue
			}
			if field.min > field.max || field.def < field.min || field.def > field.max {
				t.Errorf("%s %s bounds/default are inconsistent: min=%d max=%d default=%d", item, name, field.min, field.max, field.def)
			}
		}
	}
}
