package moauth

import (
	"errors"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/oauth"
	"mosona-manager/internal/security/oauthprofile"
	"strings"
)

type providerInput struct {
	Name         string
	Icon         string
	Protocol     string
	IssuerURL    string
	AuthURL      string
	TokenURL     string
	UserinfoURL  string
	Scopes       string
	SubjectField string
	ClientID     string
	ClientSecret string
	Skip2FA      bool
	IsEnabled    bool
}

func (input providerInput) normalize() (providerInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Protocol = strings.ToLower(strings.TrimSpace(input.Protocol))
	input.IssuerURL = strings.TrimSpace(input.IssuerURL)
	input.AuthURL = strings.TrimSpace(input.AuthURL)
	input.TokenURL = strings.TrimSpace(input.TokenURL)
	input.UserinfoURL = strings.TrimSpace(input.UserinfoURL)
	input.Scopes = strings.Join(strings.Fields(input.Scopes), " ")
	input.SubjectField = strings.TrimSpace(input.SubjectField)
	input.ClientID = strings.TrimSpace(input.ClientID)

	if input.Protocol == "" {
		input.Protocol = oauth.ProtocolOAuth2
	}
	if input.Name == "" || input.ClientID == "" || input.ClientSecret == "" {
		return providerInput{}, errors.New("missing required fields")
	}

	switch input.Protocol {
	case oauth.ProtocolOAuth2:
		if input.AuthURL == "" || input.TokenURL == "" || input.UserinfoURL == "" {
			return providerInput{}, errors.New("missing OAuth2 endpoint")
		}
		if input.SubjectField == "" {
			input.SubjectField = "id"
		}
		if err := oauthprofile.ValidateOAuth2SubjectField(input.SubjectField); err != nil {
			return providerInput{}, err
		}
		if input.Scopes == "" {
			input.Scopes = "read:user read:email"
		}
		input.IssuerURL = ""
	case oauth.ProtocolOIDC:
		if input.IssuerURL == "" {
			return providerInput{}, errors.New("missing OIDC issuer")
		}
		if input.Scopes == "" {
			input.Scopes = "openid profile email"
		}
		input.AuthURL = ""
		input.TokenURL = ""
		input.UserinfoURL = ""
		input.SubjectField = "sub"
	default:
		return providerInput{}, errors.New("invalid provider protocol")
	}
	return input, nil
}

func (input providerInput) provider(id int) _type.AuthProvider {
	return _type.AuthProvider{
		ID:           id,
		Name:         input.Name,
		Icon:         input.Icon,
		Protocol:     input.Protocol,
		IssuerUrl:    input.IssuerURL,
		AuthUrl:      input.AuthURL,
		TokenUrl:     input.TokenURL,
		UserinfoUrl:  input.UserinfoURL,
		Scopes:       input.Scopes,
		SubjectField: input.SubjectField,
		ClientID:     input.ClientID,
		ClientSecret: input.ClientSecret,
		Skip2FA:      input.Skip2FA,
		IsEnabled:    input.IsEnabled,
	}
}
