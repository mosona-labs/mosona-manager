package config

import "testing"

func TestParseSecureCookies(t *testing.T) {
	tests := []struct {
		value string
		want  bool
		ok    bool
	}{
		{value: "true", want: true, ok: true},
		{value: "false", want: false, ok: true},
		{value: "TRUE"},
		{value: "1"},
		{value: ""},
		{value: " false"},
	}

	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			got, err := parseSecureCookies(test.value)
			if (err == nil) != test.ok {
				t.Fatalf("parseSecureCookies(%q) error = %v, want success %t", test.value, err, test.ok)
			}
			if err == nil && got != test.want {
				t.Fatalf("parseSecureCookies(%q) = %t, want %t", test.value, got, test.want)
			}
		})
	}
}

func TestRedisPasswordFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		values  map[string]string
		want    string
		wantErr bool
	}{
		{name: "unset", values: map[string]string{}},
		{name: "canonical", values: map[string]string{"REDIS_PASSWORD": "canonical"}, want: "canonical"},
		{name: "legacy", values: map[string]string{"REDIS_PASS": "legacy"}, want: "legacy"},
		{name: "matching", values: map[string]string{"REDIS_PASSWORD": "same", "REDIS_PASS": "same"}, want: "same"},
		{name: "matching empty", values: map[string]string{"REDIS_PASSWORD": "", "REDIS_PASS": ""}},
		{name: "conflict", values: map[string]string{"REDIS_PASSWORD": "canonical", "REDIS_PASS": "legacy"}, wantErr: true},
		{name: "explicit empty conflict", values: map[string]string{"REDIS_PASSWORD": "", "REDIS_PASS": "legacy"}, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := redisPasswordFromEnv(func(key string) (string, bool) {
				value, ok := test.values[key]
				return value, ok
			})
			if (err != nil) != test.wantErr {
				t.Fatalf("redisPasswordFromEnv() error = %v, wantErr %t", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("redisPasswordFromEnv() = %q, want %q", got, test.want)
			}
		})
	}
}
