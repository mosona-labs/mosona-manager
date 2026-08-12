package db

import (
	"database/sql"
	"errors"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/security/oauthprofile"

	"github.com/lib/pq"
)

func GetOAuthProvider() ([]_type.AuthProvider, error) {
	var providers = make([]_type.AuthProvider, 0)
	if err := Db.Select(
		&providers,
		"SELECT id, name, icon, protocol, issuer_url, auth_url, token_url, userinfo_url, scopes, subject_field, identity_namespace_version, config_revision, client_id, client_secret, skip_2fa, is_enabled, sort, created_at, updated_at FROM auth_provider ORDER BY sort, id DESC",
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

func GetAuthIdentityBySubject(providerID int, expectedVersion, expectedRevision int64, subject string) (*_type.AuthIdentity, error) {
	tx, err := Db.Beginx()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var currentVersion int64
	var currentRevision int64
	var isEnabled bool
	if err = tx.QueryRow(
		"SELECT identity_namespace_version, config_revision, is_enabled FROM auth_provider WHERE id=$1 FOR SHARE",
		providerID,
	).Scan(&currentVersion, &currentRevision, &isEnabled); err != nil {
		return nil, err
	}
	if currentVersion != expectedVersion || currentRevision != expectedRevision || !isEnabled {
		return nil, ErrOAuthIdentityNamespaceChanged
	}

	var identity _type.AuthIdentity
	if err = tx.Get(
		&identity,
		"SELECT id, user_id, provider_id, subject, email, name, quarantined FROM auth_identity WHERE provider_id=$1 AND subject=$2 AND quarantined=false AND subject<>'0'",
		providerID,
		subject,
	); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &identity, nil
}

func GetAuthProviderByID(id int) (*_type.AuthProvider, error) {
	var provider _type.AuthProvider
	if err := Db.Get(
		&provider,
		"SELECT id, name, icon, protocol, issuer_url, auth_url, token_url, userinfo_url, scopes, subject_field, identity_namespace_version, config_revision, client_id, client_secret, skip_2fa, is_enabled, sort, created_at, updated_at FROM auth_provider WHERE id=$1",
		id,
	); err != nil {
		return nil, err
	}
	return &provider, nil
}

func CreateOAuthProvider(name, icon, protocol, issuerURL, authURL, tokenURL, userinfoURL, scopes, subjectField, clientID, clientSecret string, skip2FA, isEnabled bool) (int, int64, int64, error) {
	var id int
	var identityNamespaceVersion int64
	var configRevision int64
	err := Db.QueryRow(
		`INSERT INTO auth_provider (name, icon, protocol, issuer_url, auth_url, token_url, userinfo_url, scopes, subject_field, client_id, client_secret, skip_2fa, is_enabled)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13) RETURNING id, identity_namespace_version, config_revision`,
		name, icon, protocol, issuerURL, authURL, tokenURL, userinfoURL, scopes, subjectField, clientID, clientSecret, skip2FA, isEnabled,
	).Scan(&id, &identityNamespaceVersion, &configRevision)
	if err != nil {
		return 0, 0, 0, err
	}
	return id, identityNamespaceVersion, configRevision, nil
}

type oauthIdentityNamespace struct {
	Protocol     string
	IssuerURL    string
	AuthURL      string
	TokenURL     string
	UserinfoURL  string
	SubjectField string
	ClientID     string
}

func UpdateOAuthProvider(id int, name, icon, protocol, issuerURL, authURL, tokenURL, userinfoURL, scopes, subjectField, clientID, clientSecret string, skip2FA, isEnabled bool) (int64, int64, error) {
	tx, err := Db.Beginx()
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback() }()

	current := oauthIdentityNamespace{}
	var currentVersion int64
	var currentRevision int64
	err = tx.QueryRow(
		`SELECT protocol, issuer_url, auth_url, token_url, userinfo_url, subject_field, client_id, identity_namespace_version, config_revision
		FROM auth_provider WHERE id=$1 FOR UPDATE`,
		id,
	).Scan(
		&current.Protocol, &current.IssuerURL, &current.AuthURL, &current.TokenURL,
		&current.UserinfoURL, &current.SubjectField, &current.ClientID, &currentVersion, &currentRevision,
	)
	if err != nil {
		return 0, 0, err
	}

	next := oauthIdentityNamespace{
		Protocol: protocol, IssuerURL: issuerURL, AuthURL: authURL, TokenURL: tokenURL,
		UserinfoURL: userinfoURL, SubjectField: subjectField, ClientID: clientID,
	}
	nextVersion := currentVersion
	namespaceChanged := current != next
	if namespaceChanged {
		var hasActiveIdentity bool
		if err = tx.QueryRow(
			"SELECT EXISTS (SELECT 1 FROM auth_identity WHERE provider_id=$1 AND quarantined=false)",
			id,
		).Scan(&hasActiveIdentity); err != nil {
			return 0, 0, err
		}
		if hasActiveIdentity {
			return 0, 0, ErrOAuthIdentityNamespaceLocked
		}
		if currentVersion == 1<<63-1 {
			return 0, 0, ErrOAuthIdentityNamespaceVersionExhausted
		}
		nextVersion++
	}

	if currentRevision == 1<<63-1 {
		return 0, 0, ErrOAuthConfigRevisionExhausted
	}
	nextRevision := currentRevision + 1
	err = tx.QueryRow(
		`UPDATE auth_provider SET name=$1, icon=$2, protocol=$3, issuer_url=$4, auth_url=$5, token_url=$6, userinfo_url=$7, scopes=$8, subject_field=$9, client_id=$10, client_secret=$11, skip_2fa=$12, is_enabled=$13, identity_namespace_version=$14, config_revision=$15, updated_at=NOW()
		WHERE id=$16 RETURNING identity_namespace_version, config_revision`,
		name, icon, protocol, issuerURL, authURL, tokenURL, userinfoURL, scopes, subjectField,
		clientID, clientSecret, skip2FA, isEnabled, nextVersion, nextRevision, id,
	).Scan(&nextVersion, &nextRevision)
	if err != nil {
		return 0, 0, err
	}
	if namespaceChanged {
		result, execErr := tx.Exec("DELETE FROM auth_identity WHERE provider_id=$1 AND quarantined=true", id)
		if execErr != nil {
			return 0, 0, execErr
		}
		if _, execErr = result.RowsAffected(); execErr != nil {
			return 0, 0, execErr
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, 0, err
	}
	return nextVersion, nextRevision, nil
}

var (
	ErrOAuthIdentityNamespaceLocked           = errors.New("OAuth identity namespace cannot change while bindings exist")
	ErrOAuthIdentityNamespaceChanged          = errors.New("OAuth identity namespace changed during authorization")
	ErrOAuthIdentityNamespaceVersionExhausted = errors.New("OAuth identity namespace version is exhausted")
	ErrOAuthConfigRevisionExhausted           = errors.New("OAuth provider config revision is exhausted")
	ErrOAuthIdentityAlreadyLinked             = errors.New("OAuth account or provider is already linked")
)

func DeleteOAuthProvider(id int) (int64, error) {
	tx, err := Db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var configRevision int64
	if err = tx.QueryRow("SELECT config_revision FROM auth_provider WHERE id=$1 FOR UPDATE", id).Scan(&configRevision); err != nil {
		return 0, err
	}

	_, err = tx.Exec("DELETE FROM auth_identity WHERE provider_id=$1", id)
	if err != nil {
		return 0, err
	}
	_, err = tx.Exec("DELETE FROM auth_provider WHERE id=$1", id)
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return configRevision, nil
}

func GetAuthByUserID(userID int64) ([]_type.PublicAuthIdentity, error) {
	rows, err := Db.Query(
		`SELECT ap.id, ap.name, ap.icon, ai.name, ai.email
		  FROM auth_provider ap
		  LEFT JOIN auth_identity ai ON ai.provider_id = ap.id AND ai.user_id = $1 AND ai.quarantined=false AND ai.subject<>'0'
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

		if err := rows.Scan(
			&identity.Id,
			&identity.Name,
			&identity.Icon,
			&name,
			&email,
		); err != nil {
			return nil, err
		}

		if name.Valid {
			identity.Linked.Name = name.String
		}
		if email.Valid {
			identity.Linked.Email = email.String
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
		"SELECT id, user_id, provider_id, subject, email, name, quarantined FROM auth_identity WHERE provider_id=$1 AND user_id=$2 AND quarantined=false AND subject<>'0'",
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
		"SELECT id, user_id, provider_id, subject, email, name, quarantined FROM auth_identity WHERE provider_id=$1 AND subject=$2 AND quarantined=false AND subject<>'0'",
		providerID,
		subject,
	); err != nil {
		return nil, err
	}
	return &identity, nil
}

func AddAuthIdentity(userID int64, providerID int, expectedVersion, expectedRevision int64, subject, email, name string) (int64, error) {
	if _, err := oauthprofile.ValidateSubject(subject); err != nil {
		return 0, err
	}
	tx, err := Db.Beginx()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var currentVersion int64
	var currentRevision int64
	var isEnabled bool
	if err = tx.QueryRow(
		"SELECT identity_namespace_version, config_revision, is_enabled FROM auth_provider WHERE id=$1 FOR UPDATE",
		providerID,
	).Scan(&currentVersion, &currentRevision, &isEnabled); err != nil {
		return 0, err
	}
	if currentVersion != expectedVersion || currentRevision != expectedRevision || !isEnabled {
		return 0, ErrOAuthIdentityNamespaceChanged
	}

	var id int64
	err = tx.QueryRow(
		`INSERT INTO auth_identity (user_id, provider_id, subject, email, name) 
			VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		userID, providerID, subject, email, name,
	).Scan(&id)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" &&
			(pqErr.Constraint == "U_OAUTH" || pqErr.Constraint == "auth_identity_active_user_provider_unique") {
			return 0, ErrOAuthIdentityAlreadyLinked
		}
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}
