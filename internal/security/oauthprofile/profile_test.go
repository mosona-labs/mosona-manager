package oauthprofile

import (
	"errors"
	"strings"
	"testing"
)

func TestDecodeOAuth2Subject(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		field     string
		subject   string
		wantError bool
	}{
		{name: "numeric id", body: `{"id":42}`, field: "id", subject: "42"},
		{name: "string id", body: `{"account_id":"github-enterprise-user"}`, field: "account_id", subject: "github-enterprise-user"},
		{name: "does not accept oidc sub", body: `{"sub":"oidc-user"}`, field: "id", wantError: true},
		{name: "sub cannot be configured for oauth2", body: `{"sub":"oidc-user"}`, field: "sub", wantError: true},
		{name: "missing subject", body: `{}`, field: "id", wantError: true},
		{name: "zero numeric id", body: `{"id":0}`, field: "id", wantError: true},
		{name: "zero string id", body: `{"id":"0"}`, field: "id", wantError: true},
		{name: "negative id", body: `{"id":-1}`, field: "id", wantError: true},
		{name: "fractional id", body: `{"id":1.5}`, field: "id", wantError: true},
		{name: "object id", body: `{"id":{"value":42}}`, field: "id", wantError: true},
		{name: "leading whitespace", body: `{"id":" user"}`, field: "id", wantError: true},
		{name: "invalid field", body: `{"id":42}`, field: "data.id", wantError: true},
		{name: "trailing value", body: `{"id":42} {}`, field: "id", wantError: true},
		{name: "invalid json", body: `{"id":`, field: "id", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile, err := DecodeOAuth2(strings.NewReader(tt.body), tt.field)
			if tt.wantError {
				if err == nil {
					t.Fatal("DecodeOAuth2() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("DecodeOAuth2() error = %v", err)
			}
			if profile.Subject != tt.subject {
				t.Fatalf("DecodeOAuth2() subject = %q, want %q", profile.Subject, tt.subject)
			}
		})
	}
}

func TestDecodeOIDCClaims(t *testing.T) {
	profile, err := DecodeOIDCClaims("subject", []byte(`{
		"sub":"untrusted-duplicate",
		"preferred_username":"login",
		"email":"user@example.com"
	}`))
	if err != nil {
		t.Fatalf("DecodeOIDCClaims() error = %v", err)
	}
	if profile.Subject != "subject" || profile.Name != "login" || profile.Email != "user@example.com" {
		t.Fatalf("DecodeOIDCClaims() profile = %#v", profile)
	}
}

func TestDecodeRejectsOversizedValues(t *testing.T) {
	t.Run("subject", func(t *testing.T) {
		_, err := DecodeOIDCClaims(strings.Repeat("a", maxSubjectBytes+1), []byte(`{}`))
		if !errors.Is(err, ErrInvalidSubject) {
			t.Fatalf("DecodeOIDCClaims() error = %v, want ErrInvalidSubject", err)
		}
	})

	t.Run("response", func(t *testing.T) {
		_, err := DecodeOAuth2(strings.NewReader(strings.Repeat(" ", maxUserInfoBytes+1)), "id")
		if err == nil {
			t.Fatal("DecodeOAuth2() error = nil, want oversized response error")
		}
	})
}
