package utils

import "testing"

func TestSiteHostFromBaseURL(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"https://manager.example.com", "manager.example.com"},
		{"http://manager.example.com:8080/path", "manager.example.com"},
		{"manager.example.com.", "manager.example.com"},
	}
	for _, tt := range tests {
		if got := SiteHostFromBaseURL(tt.in); got != tt.want {
			t.Fatalf("SiteHostFromBaseURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRequestHostMatchesBaseURL(t *testing.T) {
	base := "https://hub.example.com"
	if !RequestHostMatchesBaseURL("hub.example.com", base) {
		t.Fatal("expected match")
	}
	if !RequestHostMatchesBaseURL("Hub.Example.com:443", base) {
		t.Fatal("expected case-insensitive match")
	}
	if RequestHostMatchesBaseURL("evil.example.com", base) {
		t.Fatal("expected mismatch")
	}
	if !RequestHostMatchesBaseURL("anything", "") {
		t.Fatal("empty base should allow")
	}
}
