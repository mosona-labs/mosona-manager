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
