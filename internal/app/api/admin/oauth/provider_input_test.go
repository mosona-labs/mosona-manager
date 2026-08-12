package moauth

import "testing"

func TestProviderInputNormalizeSeparatesOAuth2AndOIDCConfiguration(t *testing.T) {
	t.Run("legacy OAuth2 defaults", func(t *testing.T) {
		input, err := (providerInput{
			Name: " Provider ", AuthURL: " https://provider.example/authorize ",
			TokenURL: " https://provider.example/token ", UserinfoURL: " https://provider.example/user ",
			ClientID: " client ", ClientSecret: "secret",
		}).normalize()
		if err != nil {
			t.Fatal(err)
		}
		if input.Protocol != "oauth2" || input.SubjectField != "id" || input.Scopes != "read:user read:email" {
			t.Fatalf("unexpected OAuth2 defaults: %#v", input)
		}
	})

	t.Run("OIDC ignores untrusted manual endpoints", func(t *testing.T) {
		input, err := (providerInput{
			Name: "Provider", Protocol: "oidc", IssuerURL: " https://issuer.example/ ",
			AuthURL: "https://attacker.example/authorize", TokenURL: "https://attacker.example/token",
			UserinfoURL: "https://attacker.example/user", Scopes: " profile   email ",
			SubjectField: "id", ClientID: "client", ClientSecret: "secret",
		}).normalize()
		if err != nil {
			t.Fatal(err)
		}
		if input.IssuerURL != "https://issuer.example/" || input.AuthURL != "" || input.TokenURL != "" || input.UserinfoURL != "" {
			t.Fatalf("OIDC endpoints were not normalized: %#v", input)
		}
		if input.SubjectField != "sub" || input.Scopes != "profile email" {
			t.Fatalf("unexpected OIDC identity settings: %#v", input)
		}
	})
}
