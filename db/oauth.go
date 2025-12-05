package db

import (
	"database/sql"
	"mosona-manager/_type"
)

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

func GetAuthByUserID(userID int64) ([]_type.PublicAuthIdentity, error) {
	rows, err := Db.Query(
		`SELECT ap.id, ap.name, ap.icon, ai.name, ai.email, ai.last_login_at
		  FROM auth_provider ap
		  LEFT JOIN auth_identity ai ON ai.provider_id = ap.id AND ai.user_id = $1
		  WHERE ap.is_enabled = true
		  ORDER BY ap.sort, ap.id DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var identities = make([]_type.PublicAuthIdentity, 0)
	for rows.Next() {
		var identity _type.PublicAuthIdentity
		var name, email sql.NullString
		var lastLoginAt sql.NullTime

		if err := rows.Scan(
			&identity.Id,
			&identity.Name,
			&identity.Icon,
			&name,
			&email,
			&lastLoginAt,
		); err != nil {
			return nil, err
		}

		if name.Valid {
			identity.Linked.Name = name.String
		}
		if email.Valid {
			identity.Linked.Email = email.String
		}
		if lastLoginAt.Valid {
			identity.Linked.LastLoginAt = lastLoginAt.Time
		}

		identities = append(identities, identity)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return identities, nil
}
func DeleteAuthIdentityByProviderAndUserID(providerID int, userID int64) error {
	_, err := Db.Exec(
		"DELETE FROM auth_identity WHERE provider_id=$1 AND user_id=$2",
		providerID,
		userID,
	)
	return err
}

func GetBindByProviderAndUserID(providerID int, userID int64) (*_type.AuthIdentity, error) {
	var identity _type.AuthIdentity
	if err := Db.Get(
		&identity,
		"SELECT id, user_id, provider_id, subject, email, name, last_login_at FROM auth_identity WHERE provider_id=$1 AND user_id=$2",
		providerID,
		userID,
	); err != nil {
		return nil, err
	}
	return &identity, nil
}

func GetBindByProviderAndSubject(providerID int, subject string) (*_type.AuthIdentity, error) {
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

func AddAuthIdentity(userID int64, providerID int, subject, email, name string) (int64, error) {
	var id int64
	err := Db.QueryRow(
		`INSERT INTO auth_identity (user_id, provider_id, subject, email, name, last_login_at) 
		VALUES ($1, $2, $3, $4, $5, NOW()) RETURNING id`,
		userID, providerID, subject, email, name,
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}
