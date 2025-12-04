package db

import "mosona-manager/_type"

func GetOAuthProvider() ([]_type.AuthProvider, error) {
	var providers = make([]_type.AuthProvider, 0)
	if err := Db.Select(
		&providers,
		"SELECT id, name, icon, auth_url, token_url, userinfo_url, client_id, client_secret, skip_2fa, is_enabled, created_at, updated_at FROM auth_provider ORDER BY sort, id DESC",
	); err != nil {
		return nil, err
	}
	return providers, nil
}

func GetAuthIdentityBySubject(providerID int, subject string) (*_type.AuthIdentity, error) {
	var identity _type.AuthIdentity
	if err := Db.Get(
		&identity,
		"SELECT id, user_id, provider_id, subject, email, name, last_login_at FROM auth_identity WHERE provider_id=$1 AND subject=$2",
		providerID,
		subject,
	); err != nil {
		return nil, err
	}
	return &identity, nil
}

func GetAuthProviderByID(id int) (*_type.AuthProvider, error) {
	var provider _type.AuthProvider
	if err := Db.Get(
		&provider,
		"SELECT id, name, icon, auth_url, token_url, userinfo_url, client_id, client_secret, skip_2fa, is_enabled, created_at, updated_at FROM auth_provider WHERE id=$1",
		id,
	); err != nil {
		return nil, err
	}
	return &provider, nil
}
