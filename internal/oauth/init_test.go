package oauth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"io"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/utils/store"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"golang.org/x/oauth2"
)

type fakeVerifier struct {
	identity VerifiedOIDCIdentity
	err      error
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func (v fakeVerifier) Verify(context.Context, string) (VerifiedOIDCIdentity, error) {
	return v.identity, v.err
}

func TestOIDCIdentityRequiresVerifiedTokenAndNonce(t *testing.T) {
	tests := []struct {
		name          string
		token         *oauth2.Token
		nonce         string
		verifier      OIDCVerifier
		wantSubject   string
		wantErrorText string
	}{
		{
			name:  "verified identity",
			token: (&oauth2.Token{AccessToken: "access"}).WithExtra(map[string]any{"id_token": "signed"}),
			nonce: "nonce",
			verifier: fakeVerifier{identity: VerifiedOIDCIdentity{
				Subject: "subject-a", Nonce: "nonce", Claims: []byte(`{"email":"a@example.com"}`),
			}},
			wantSubject: "subject-a",
		},
		{
			name: "missing id token", token: &oauth2.Token{AccessToken: "access"}, nonce: "nonce",
			verifier: fakeVerifier{}, wantErrorText: "no ID Token",
		},
		{
			name:  "signature issuer audience or expiry verification fails",
			token: (&oauth2.Token{AccessToken: "access"}).WithExtra(map[string]any{"id_token": "bad"}),
			nonce: "nonce", verifier: fakeVerifier{err: errors.New("signature verification failed")},
			wantErrorText: "verify OIDC ID Token",
		},
		{
			name:  "nonce mismatch",
			token: (&oauth2.Token{AccessToken: "access"}).WithExtra(map[string]any{"id_token": "signed"}),
			nonce: "expected", verifier: fakeVerifier{identity: VerifiedOIDCIdentity{
				Subject: "subject-a", Nonce: "other", Claims: []byte(`{}`),
			}},
			wantErrorText: "nonce did not match",
		},
		{
			name:  "zero subject",
			token: (&oauth2.Token{AccessToken: "access"}).WithExtra(map[string]any{"id_token": "signed"}),
			nonce: "nonce", verifier: fakeVerifier{identity: VerifiedOIDCIdentity{
				Subject: "0", Nonce: "nonce", Claims: []byte(`{}`),
			}},
			wantErrorText: "valid subject",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ProviderConfig{Protocol: ProtocolOIDC, OIDC: tt.verifier}
			profile, err := cfg.Identity(context.Background(), tt.token, tt.nonce)
			if tt.wantErrorText != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrorText) {
					t.Fatalf("Identity() error = %v, want containing %q", err, tt.wantErrorText)
				}
				return
			}
			if err != nil {
				t.Fatalf("Identity() error = %v", err)
			}
			if profile.Subject != tt.wantSubject {
				t.Fatalf("Identity() subject = %q, want %q", profile.Subject, tt.wantSubject)
			}
		})
	}
}

func TestOIDCVerifierValidatesCoreClaims(t *testing.T) {
	trustedKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	untrustedKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const issuer = "https://issuer.example.com"
	const clientID = "mosona-client"
	verifier := oidcVerifier{verifier: oidc.NewVerifier(issuer, &oidc.StaticKeySet{
		PublicKeys: []crypto.PublicKey{&trustedKey.PublicKey},
	}, &oidc.Config{ClientID: clientID})}

	tests := []struct {
		name       string
		key        *rsa.PrivateKey
		issuer     string
		audience   string
		expiry     time.Time
		nonce      string
		wantVerify bool
		wantNonce  string
	}{
		{name: "valid", key: trustedKey, issuer: issuer, audience: clientID, expiry: time.Now().Add(time.Hour), nonce: "nonce", wantVerify: true, wantNonce: "nonce"},
		{name: "wrong signature", key: untrustedKey, issuer: issuer, audience: clientID, expiry: time.Now().Add(time.Hour), nonce: "nonce"},
		{name: "wrong issuer", key: trustedKey, issuer: "https://attacker.example.com", audience: clientID, expiry: time.Now().Add(time.Hour), nonce: "nonce"},
		{name: "wrong audience", key: trustedKey, issuer: issuer, audience: "other-client", expiry: time.Now().Add(time.Hour), nonce: "nonce"},
		{name: "expired", key: trustedKey, issuer: issuer, audience: clientID, expiry: time.Now().Add(-time.Hour), nonce: "nonce"},
		{name: "nonce is returned for caller validation", key: trustedKey, issuer: issuer, audience: clientID, expiry: time.Now().Add(time.Hour), nonce: "different", wantVerify: true, wantNonce: "different"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := signIDToken(t, tt.key, map[string]any{
				"iss": tt.issuer, "sub": "subject-a", "aud": tt.audience,
				"exp": tt.expiry.Unix(), "iat": time.Now().Add(-time.Minute).Unix(), "nonce": tt.nonce,
			})
			identity, err := verifier.Verify(context.Background(), raw)
			if !tt.wantVerify {
				if err == nil {
					t.Fatal("Verify() error = nil, want verification failure")
				}
				return
			}
			if err != nil {
				t.Fatalf("Verify() error = %v", err)
			}
			if identity.Subject != "subject-a" || identity.Nonce != tt.wantNonce {
				t.Fatalf("Verify() identity = %#v", identity)
			}
		})
	}
}

func signIDToken(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	signed, err := signer.Sign(payload)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := signed.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestOAuth2IdentityUsesConfiguredLegacyField(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Authorization") != "Bearer access-token" {
			t.Fatalf("Authorization = %q", request.Header.Get("Authorization"))
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"sub":"must-not-win","account_id":123,"email":"a@example.com"}`)),
			Request:    request,
		}, nil
	})}

	cfg := &ProviderConfig{
		Config: &oauth2.Config{}, Protocol: ProtocolOAuth2,
		UserinfoURL: "https://provider.example.com/user", SubjectField: "account_id", HTTPClient: httpClient,
	}
	profile, err := cfg.Identity(context.Background(), &oauth2.Token{AccessToken: "access-token", TokenType: "Bearer"}, "")
	if err != nil {
		t.Fatalf("Identity() error = %v", err)
	}
	if profile.Subject != "123" {
		t.Fatalf("Identity() subject = %q, want 123", profile.Subject)
	}
}

func TestBuildOAuth2ProviderPreservesLegacyDefaults(t *testing.T) {
	cfg, err := BuildProvider(context.Background(), _type.AuthProvider{
		AuthUrl: "https://example.com/authorize", TokenUrl: "https://example.com/token",
		UserinfoUrl: "https://example.com/user", ClientID: "client", ClientSecret: "secret",
	})
	if err != nil {
		t.Fatalf("BuildProvider() error = %v", err)
	}
	if cfg.Protocol != ProtocolOAuth2 || cfg.SubjectField != "id" {
		t.Fatalf("BuildProvider() = protocol %q field %q", cfg.Protocol, cfg.SubjectField)
	}
	if cfg.IdentityNamespaceVersion != 1 {
		t.Fatalf("identity namespace version = %d, want legacy default 1", cfg.IdentityNamespaceVersion)
	}
	if cfg.ConfigRevision != 1 {
		t.Fatalf("config revision = %d, want legacy default 1", cfg.ConfigRevision)
	}
	if got, _ := json.Marshal(cfg.Config.Scopes); string(got) != `["read:user","read:email"]` {
		t.Fatalf("scopes = %s", got)
	}
}

func TestInstallProviderRejectsStaleOrDuplicateRevision(t *testing.T) {
	const providerID = 901
	clearTestProvider(providerID)
	t.Cleanup(func() { clearTestProvider(providerID) })

	InstallProvider(providerID, testProviderConfig("new", 3))
	InstallProvider(providerID, testProviderConfig("old", 2))
	InstallProvider(providerID, testProviderConfig("duplicate", 3))

	cfg, ok := GetProviders(providerID)
	if !ok || cfg.SubjectField != "new" || cfg.ConfigRevision != 3 {
		t.Fatalf("installed provider = %#v, ok = %v", cfg, ok)
	}
}

func TestRemoveProviderDoesNotDeleteNewerRevision(t *testing.T) {
	const providerID = 902
	clearTestProvider(providerID)
	t.Cleanup(func() { clearTestProvider(providerID) })

	InstallProvider(providerID, testProviderConfig("new", 4))
	RemoveProvider(providerID, 3)
	if _, ok := GetProviders(providerID); !ok {
		t.Fatal("stale delete removed a newer provider config")
	}
	RemoveProvider(providerID, 4)
	if _, ok := GetProviders(providerID); ok {
		t.Fatal("matching delete did not remove provider config")
	}
}

func TestRemoveProviderRevokesPendingAuthorizationState(t *testing.T) {
	const providerID = 906
	clearTestProvider(providerID)
	t.Cleanup(func() { clearTestProvider(providerID) })

	InstallProvider(providerID, testProviderConfig("enabled", 3))
	if _, ok := BeginAuthorization(providerID, "state-disabled"); !ok {
		t.Fatal("expected enabled provider to start authorization")
	}
	RemoveProvider(providerID, 3)

	if store.ConsumeAuthSessionState("state-disabled", providerID, 3, time.Now()) {
		t.Fatal("disabled provider left pending authorization state active")
	}
	if _, ok := BeginAuthorization(providerID, "state-after-disable"); ok {
		t.Fatal("disabled provider started a new authorization")
	}
}

func TestProviderRevisionUpdateRevokesOnlyOlderAuthorizationState(t *testing.T) {
	const providerID = 907
	clearTestProvider(providerID)
	t.Cleanup(func() { clearTestProvider(providerID) })

	InstallProvider(providerID, testProviderConfig("old", 3))
	if _, ok := BeginAuthorization(providerID, "state-old-revision"); !ok {
		t.Fatal("expected old provider revision to start authorization")
	}
	InstallProvider(providerID, testProviderConfig("new", 4))
	if store.ConsumeAuthSessionState("state-old-revision", providerID, 3, time.Now()) {
		t.Fatal("provider update left old authorization state active")
	}
	if _, ok := BeginAuthorization(providerID, "state-new-revision"); !ok {
		t.Fatal("expected new provider revision to start authorization")
	}
	RemoveProvider(providerID, 3)
	if !store.ConsumeAuthSessionState("state-new-revision", providerID, 4, time.Now()) {
		t.Fatal("stale provider removal revoked newer authorization state")
	}
}

func TestMergeProviderSnapshotCannotRestoreStaleConfig(t *testing.T) {
	const providerID = 903
	clearTestProvider(providerID)
	t.Cleanup(func() { clearTestProvider(providerID) })

	InstallProvider(providerID, testProviderConfig("new", 3))
	mergeProviderSnapshot(map[int]*ProviderConfig{providerID: testProviderConfig("old", 2)}, map[int]int64{providerID: 2})
	cfg, ok := GetProviders(providerID)
	if !ok || cfg.SubjectField != "new" {
		t.Fatalf("stale snapshot replaced newer config: %#v, ok = %v", cfg, ok)
	}

	RemoveProvider(providerID, 3)
	mergeProviderSnapshot(map[int]*ProviderConfig{providerID: testProviderConfig("deleted", 3)}, map[int]int64{providerID: 3})
	if _, ok := GetProviders(providerID); ok {
		t.Fatal("stale snapshot restored a deleted provider")
	}
}

func TestGetProvidersReturnsIndependentOAuthConfig(t *testing.T) {
	const providerID = 904
	clearTestProvider(providerID)
	t.Cleanup(func() { clearTestProvider(providerID) })

	cfg := testProviderConfig("id", 1)
	cfg.Config.Scopes = []string{"first"}
	InstallProvider(providerID, cfg)
	first, _ := GetProviders(providerID)
	first.Config.RedirectURL = "changed"
	first.Config.Scopes[0] = "changed"
	second, _ := GetProviders(providerID)
	if second.Config.RedirectURL == "changed" || second.Config.Scopes[0] == "changed" {
		t.Fatalf("provider config was shared across readers: %#v", second.Config)
	}
}

func TestMergeProviderSnapshotRemovesInvalidOrDisabledRevision(t *testing.T) {
	const providerID = 905
	clearTestProvider(providerID)
	t.Cleanup(func() { clearTestProvider(providerID) })

	InstallProvider(providerID, testProviderConfig("old", 2))
	mergeProviderSnapshot(map[int]*ProviderConfig{}, map[int]int64{providerID: 3})
	if _, ok := GetProviders(providerID); ok {
		t.Fatal("invalid or disabled DB revision left an older provider config active")
	}
}

func testProviderConfig(subjectField string, revision int64) *ProviderConfig {
	return &ProviderConfig{
		Config:         &oauth2.Config{},
		Protocol:       ProtocolOAuth2,
		SubjectField:   subjectField,
		ConfigRevision: revision,
	}
}

func clearTestProvider(providerID int) {
	providerLock.Lock()
	delete(configs, providerID)
	delete(deletedRevisions, providerID)
	providerLock.Unlock()
}
