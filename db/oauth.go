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

func GetOAuthList() ([]_type.PublicAuthProvider, error) {
	var providers = make([]_type.PublicAuthProvider, 0)
	if err := Db.Select(
		&providers,
		"SELECT id, name, icon FROM auth_provider WHERE is_enabled ORDER BY sort, id DESC",
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

func CreateOAuthProvider(name, icon, authUrl, tokenUrl, userinfoUrl, clientID, clientSecret string, skip2FA, isEnabled bool) (int, error) {
	var id int
	err := Db.QueryRow(
		`INSERT INTO auth_provider (name, icon, auth_url, token_url, userinfo_url, client_id, client_secret, skip_2fa, is_enabled) 
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
		name, icon, authUrl, tokenUrl, userinfoUrl, clientID, clientSecret, skip2FA, isEnabled,
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func UpdateOAuthProvider(id int, name, icon, authUrl, tokenUrl, userinfoUrl, clientID, clientSecret string, skip2FA, isEnabled bool) error {
	_, err := Db.Exec(
		`UPDATE auth_provider SET name=$1, icon=$2, auth_url=$3, token_url=$4, userinfo_url=$5, client_id=$6, client_secret=$7, skip_2fa=$8, is_enabled=$9, updated_at=NOW() WHERE id=$10`,
		name, icon, authUrl, tokenUrl, userinfoUrl, clientID, clientSecret, skip2FA, isEnabled, id,
	)
	return err
}

func DeleteOAuthProvider(id int) error {
	tx, err := Db.Begin()
	if err != nil {
		return err
	}

	_, err = tx.Exec("DELETE FROM auth_identity WHERE provider_id=$1", id)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	_, err = tx.Exec("DELETE FROM auth_provider WHERE id=$1", id)
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}
