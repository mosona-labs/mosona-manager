package oauth

import (
	"context"
	"errors"
	"fmt"
	"log"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/config"
	"mosona-manager/internal/db"
	"mosona-manager/internal/security/oauthprofile"
	"mosona-manager/internal/utils/store"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const (
	ProtocolOAuth2 = "oauth2"
	ProtocolOIDC   = "oidc"
)

var ErrInvalidProvider = errors.New("invalid OAuth provider configuration")

type ProviderConfig struct {
	Config                   *oauth2.Config
	Protocol                 string
	UserinfoURL              string
	SubjectField             string
	IdentityNamespaceVersion int64
	ConfigRevision           int64
	Skip                     bool
	OIDC                     OIDCVerifier
	HTTPClient               *http.Client
}

type OIDCVerifier interface {
	Verify(context.Context, string) (VerifiedOIDCIdentity, error)
}

type VerifiedOIDCIdentity struct {
	Subject string
	Nonce   string
	Claims  []byte
}

type oidcVerifier struct {
	verifier *oidc.IDTokenVerifier
}

func (v oidcVerifier) Verify(ctx context.Context, raw string) (VerifiedOIDCIdentity, error) {
	token, err := v.verifier.Verify(ctx, raw)
	if err != nil {
		return VerifiedOIDCIdentity{}, err
	}
	var claims jsonClaims
	if err = token.Claims(&claims); err != nil {
		return VerifiedOIDCIdentity{}, err
	}
	return VerifiedOIDCIdentity{Subject: token.Subject, Nonce: token.Nonce, Claims: claims.Raw}, nil
}

var (
	configs          = make(map[int]*ProviderConfig)
	deletedRevisions = make(map[int]int64)
	providerLock     sync.RWMutex
)

func Init() {
	providerList, err := db.GetOAuthProvider()
	if err != nil {
		log.Fatalln("Init auth provider error:", err)
	}

	tempConfigs := make(map[int]*ProviderConfig)
	providerRevisions := make(map[int]int64, len(providerList))
	for _, provider := range providerList {
		providerRevisions[provider.ID] = provider.ConfigRevision
		if !provider.IsEnabled {
			continue
		}
		cfg, err := BuildProvider(context.Background(), provider)
		if err != nil {
			log.Printf("Skipping invalid auth provider %d (%s): %v", provider.ID, provider.Name, err)
			continue
		}
		tempConfigs[provider.ID] = cfg
	}

	mergeProviderSnapshot(tempConfigs, providerRevisions)
}

func mergeProviderSnapshot(tempConfigs map[int]*ProviderConfig, providerRevisions map[int]int64) {
	providerLock.Lock()
	revocations := make(map[int]int64)
	for providerID, revision := range providerRevisions {
		if _, ok := tempConfigs[providerID]; !ok && deletedRevisions[providerID] < revision {
			deletedRevisions[providerID] = revision
			revocations[providerID] = revision
		}
	}
	for providerID, cfg := range tempConfigs {
		if installProviderLocked(providerID, cfg) && cfg.ConfigRevision > 1 {
			if revision := cfg.ConfigRevision - 1; revocations[providerID] < revision {
				revocations[providerID] = revision
			}
		}
	}
	for providerID, current := range configs {
		if _, ok := tempConfigs[providerID]; !ok && current.ConfigRevision <= deletedRevisions[providerID] {
			delete(configs, providerID)
		}
	}
	providerLock.Unlock()
	for providerID, revision := range revocations {
		store.RevokeAuthSessionStates(providerID, revision)
	}
}

func RefreshRedirectURLs() {
	domain := config.ReadDynamicConf().Domain
	providerLock.Lock()
	for providerID, cfg := range configs {
		updated := cloneProviderConfig(cfg)
		updated.Config.RedirectURL = fmt.Sprintf("%s/oauth/%d", domain, providerID)
		configs[providerID] = updated
	}
	providerLock.Unlock()
}

func InstallProvider(providerID int, cfg *ProviderConfig) {
	if cfg == nil || cfg.Config == nil {
		return
	}
	cfg = cloneProviderConfig(cfg)
	cfg.Config.RedirectURL = fmt.Sprintf("%s/oauth/%d", config.ReadDynamicConf().Domain, providerID)
	providerLock.Lock()
	if deletedRevisions[providerID] < cfg.ConfigRevision {
		delete(deletedRevisions, providerID)
	}
	installed := installProviderLocked(providerID, cfg)
	providerLock.Unlock()
	if installed && cfg.ConfigRevision > 1 {
		store.RevokeAuthSessionStates(providerID, cfg.ConfigRevision-1)
	}
}

func installProviderLocked(providerID int, cfg *ProviderConfig) bool {
	if deletedRevisions[providerID] >= cfg.ConfigRevision {
		return false
	}
	if current, ok := configs[providerID]; ok && current.ConfigRevision >= cfg.ConfigRevision {
		return false
	}
	configs[providerID] = cfg
	return true
}

func BuildProvider(ctx context.Context, provider _type.AuthProvider) (*ProviderConfig, error) {
	httpClient := newProviderHTTPClient()
	identityNamespaceVersion := provider.IdentityNamespaceVersion
	if identityNamespaceVersion <= 0 {
		identityNamespaceVersion = 1
	}
	configRevision := provider.ConfigRevision
	if configRevision <= 0 {
		configRevision = 1
	}
	protocol := strings.ToLower(strings.TrimSpace(provider.Protocol))
	if protocol == "" {
		protocol = ProtocolOAuth2
	}
	redirectURL := fmt.Sprintf("%s/oauth/%d", config.ReadDynamicConf().Domain, provider.ID)

	switch protocol {
	case ProtocolOAuth2:
		subjectField := strings.TrimSpace(provider.SubjectField)
		if subjectField == "" {
			subjectField = "id"
		}
		if err := oauthprofile.ValidateOAuth2SubjectField(subjectField); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidProvider, err)
		}
		scopes := strings.Fields(provider.Scopes)
		if len(scopes) == 0 {
			scopes = []string{"read:user", "read:email"}
		}
		if provider.AuthUrl == "" || provider.TokenUrl == "" || provider.UserinfoUrl == "" || provider.ClientID == "" {
			return nil, fmt.Errorf("%w: missing OAuth2 endpoint or client ID", ErrInvalidProvider)
		}
		return &ProviderConfig{
			Config: &oauth2.Config{
				ClientID:     provider.ClientID,
				ClientSecret: provider.ClientSecret,
				Endpoint: oauth2.Endpoint{
					AuthURL:  provider.AuthUrl,
					TokenURL: provider.TokenUrl,
				},
				RedirectURL: redirectURL,
				Scopes:      scopes,
			},
			Protocol:                 ProtocolOAuth2,
			UserinfoURL:              provider.UserinfoUrl,
			SubjectField:             subjectField,
			IdentityNamespaceVersion: identityNamespaceVersion,
			ConfigRevision:           configRevision,
			Skip:                     provider.Skip2FA,
			HTTPClient:               httpClient,
		}, nil

	case ProtocolOIDC:
		issuer := strings.TrimSpace(provider.IssuerUrl)
		if issuer == "" || provider.ClientID == "" || provider.ClientSecret == "" {
			return nil, fmt.Errorf("%w: missing OIDC issuer or client credentials", ErrInvalidProvider)
		}
		discoveryCtx, cancel := context.WithTimeout(oidc.ClientContext(ctx, httpClient), 15*time.Second)
		defer cancel()
		oidcProvider, err := oidc.NewProvider(discoveryCtx, issuer)
		if err != nil {
			return nil, fmt.Errorf("discover OIDC provider: %w", err)
		}
		scopes := strings.Fields(provider.Scopes)
		if len(scopes) == 0 {
			scopes = []string{oidc.ScopeOpenID, "profile", "email"}
		}
		if !contains(scopes, oidc.ScopeOpenID) {
			scopes = append([]string{oidc.ScopeOpenID}, scopes...)
		}
		return &ProviderConfig{
			Config: &oauth2.Config{
				ClientID:     provider.ClientID,
				ClientSecret: provider.ClientSecret,
				Endpoint:     oidcProvider.Endpoint(),
				RedirectURL:  redirectURL,
				Scopes:       scopes,
			},
			Protocol:                 ProtocolOIDC,
			UserinfoURL:              oidcProvider.UserInfoEndpoint(),
			IdentityNamespaceVersion: identityNamespaceVersion,
			ConfigRevision:           configRevision,
			Skip:                     provider.Skip2FA,
			OIDC:                     oidcVerifier{verifier: oidcProvider.Verifier(&oidc.Config{ClientID: provider.ClientID})},
			HTTPClient:               httpClient,
		}, nil

	default:
		return nil, fmt.Errorf("%w: unsupported protocol %q", ErrInvalidProvider, protocol)
	}
}

func (p *ProviderConfig) AuthCodeURL(state, nonce string) string {
	if p.Protocol == ProtocolOIDC {
		return p.Config.AuthCodeURL(state, oauth2.AccessTypeOnline, oidc.Nonce(nonce))
	}
	return p.Config.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

func (p *ProviderConfig) Identity(ctx context.Context, token *oauth2.Token, expectedNonce string) (oauthprofile.Profile, error) {
	if p.Protocol == ProtocolOIDC {
		return p.oidcIdentity(ctx, token, expectedNonce)
	}
	return p.oauth2Identity(ctx, token)
}

func (p *ProviderConfig) oauth2Identity(ctx context.Context, token *oauth2.Token) (oauthprofile.Profile, error) {
	if p.Config == nil {
		return oauthprofile.Profile{}, errors.New("missing OAuth2 configuration")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.UserinfoURL, nil)
	if err != nil {
		return oauthprofile.Profile{}, fmt.Errorf("create UserInfo request: %w", err)
	}
	httpClient := p.HTTPClient
	if httpClient == nil {
		httpClient = newProviderHTTPClient()
	}
	oauthCtx := context.WithValue(ctx, oauth2.HTTPClient, httpClient)
	response, err := p.Config.Client(oauthCtx, token).Do(request)
	if err != nil {
		return oauthprofile.Profile{}, fmt.Errorf("request UserInfo: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return oauthprofile.Profile{}, fmt.Errorf("UserInfo returned HTTP %d", response.StatusCode)
	}
	return oauthprofile.DecodeOAuth2(response.Body, p.SubjectField)
}

func newProviderHTTPClient() *http.Client {
	return &http.Client{Timeout: 15 * time.Second}
}

func (p *ProviderConfig) oidcIdentity(ctx context.Context, token *oauth2.Token, expectedNonce string) (oauthprofile.Profile, error) {
	if p.OIDC == nil || expectedNonce == "" {
		return oauthprofile.Profile{}, errors.New("missing OIDC verifier or nonce")
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return oauthprofile.Profile{}, errors.New("OIDC token response has no ID Token")
	}
	identity, err := p.OIDC.Verify(ctx, rawIDToken)
	if err != nil {
		return oauthprofile.Profile{}, fmt.Errorf("verify OIDC ID Token: %w", err)
	}
	if identity.Nonce != expectedNonce {
		return oauthprofile.Profile{}, errors.New("OIDC nonce did not match")
	}
	return oauthprofile.DecodeOIDCClaims(identity.Subject, identity.Claims)
}

type jsonClaims struct {
	Raw []byte
}

func (c *jsonClaims) UnmarshalJSON(data []byte) error {
	c.Raw = append(c.Raw[:0], data...)
	return nil
}

func RemoveProvider(providerID int, configRevision int64) {
	providerLock.Lock()
	if deletedRevisions[providerID] < configRevision {
		deletedRevisions[providerID] = configRevision
	}
	if current, ok := configs[providerID]; ok && current.ConfigRevision <= configRevision {
		delete(configs, providerID)
	}
	providerLock.Unlock()
	store.RevokeAuthSessionStates(providerID, configRevision)
}

func BeginAuthorization(providerID int, state string) (*ProviderConfig, bool) {
	providerLock.RLock()
	defer providerLock.RUnlock()
	conf, ok := configs[providerID]
	if !ok {
		return nil, false
	}
	store.SetAuthSessionState(state, providerID, conf.ConfigRevision)
	return cloneProviderConfig(conf), true
}

func GetProviders(providerID int) (*ProviderConfig, bool) {
	providerLock.RLock()
	defer providerLock.RUnlock()
	conf, ok := configs[providerID]
	if !ok {
		return nil, false
	}
	return cloneProviderConfig(conf), true
}

func cloneProviderConfig(cfg *ProviderConfig) *ProviderConfig {
	if cfg == nil {
		return nil
	}
	clone := *cfg
	if cfg.Config != nil {
		oauthConfig := *cfg.Config
		oauthConfig.Scopes = append([]string(nil), cfg.Config.Scopes...)
		clone.Config = &oauthConfig
	}
	return &clone
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
