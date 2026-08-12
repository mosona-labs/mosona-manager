package db

import (
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

func TestGetAuthIdentityBySubjectExcludesQuarantinedIdentity(t *testing.T) {
	mock := setOAuthMockDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT identity_namespace_version, config_revision, is_enabled FROM auth_provider").
		WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"identity_namespace_version", "config_revision", "is_enabled"}).AddRow(3, 8, true))
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT id, user_id, provider_id, subject, email, name, quarantined FROM auth_identity WHERE provider_id=$1 AND subject=$2 AND quarantined=false AND subject<>'0'",
	)).WithArgs(7, "subject-a").WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	_, err := GetAuthIdentityBySubject(7, 3, 8, "subject-a")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetAuthIdentityBySubject() error = %v, want sql.ErrNoRows", err)
	}
}

func TestAddAuthIdentityRejectsUnsafeSubjectBeforeDatabase(t *testing.T) {
	mock := setOAuthMockDB(t)
	for _, subject := range []string{"", "0", " subject", "subject ", strings.Repeat("a", 256)} {
		if _, err := AddAuthIdentity(42, 7, 1, 1, subject, "a@example.com", "A"); err == nil {
			t.Fatalf("AddAuthIdentity(subject=%q) error = nil", subject)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAddAuthIdentityKeepsDistinctOIDCSubjects(t *testing.T) {
	mock := setOAuthMockDB(t)
	query := regexp.QuoteMeta(`INSERT INTO auth_identity (user_id, provider_id, subject, email, name) 
		VALUES ($1, $2, $3, $4, $5) RETURNING id`)
	expectIdentityInsert(mock, query, 42, 7, 4, "subject-a", "a@example.com", "A", 101)
	expectIdentityInsert(mock, query, 43, 7, 4, "subject-b", "b@example.com", "B", 102)

	first, err := AddAuthIdentity(42, 7, 4, 11, "subject-a", "a@example.com", "A")
	if err != nil {
		t.Fatal(err)
	}
	second, err := AddAuthIdentity(43, 7, 4, 11, "subject-b", "b@example.com", "B")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("distinct subjects mapped to the same identity ID %d", first)
	}
}

func TestUpdateOAuthProviderRejectsIdentityNamespaceChange(t *testing.T) {
	mock := setOAuthMockDB(t)
	expectProviderLock(mock, 7, oauthIdentityNamespace{Protocol: "oauth2", AuthURL: "https://old.example/auth", TokenURL: "https://old.example/token", UserinfoURL: "https://old.example/user", SubjectField: "id", ClientID: "old-client"}, 2)
	mock.ExpectQuery("SELECT EXISTS").WithArgs(7).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectRollback()

	_, _, err := UpdateOAuthProvider(7, "Provider", "icon", "oidc", "https://issuer.example", "", "", "", "openid", "sub", "client", "secret", false, true)
	if !errors.Is(err, ErrOAuthIdentityNamespaceLocked) {
		t.Fatalf("UpdateOAuthProvider() error = %v, want ErrOAuthIdentityNamespaceLocked", err)
	}
}

func TestUpdateOAuth2ProviderRejectsEndpointNamespaceChange(t *testing.T) {
	mock := setOAuthMockDB(t)
	expectProviderLock(mock, 7, oauthIdentityNamespace{Protocol: "oauth2", AuthURL: "https://old.example/auth", TokenURL: "https://old.example/token", UserinfoURL: "https://old.example/user", SubjectField: "id", ClientID: "client"}, 8)
	mock.ExpectQuery("SELECT EXISTS").WithArgs(7).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectRollback()

	_, _, err := UpdateOAuthProvider(7, "Provider", "icon", "oauth2", "", "https://new.example/auth", "https://new.example/token", "https://new.example/user", "read:user", "id", "client", "secret", false, true)
	if !errors.Is(err, ErrOAuthIdentityNamespaceLocked) {
		t.Fatalf("UpdateOAuthProvider() error = %v, want ErrOAuthIdentityNamespaceLocked", err)
	}
}

func TestUpdateOAuthProviderAllowsNamespaceChangeWithOnlyQuarantinedIdentities(t *testing.T) {
	mock := setOAuthMockDB(t)
	expectProviderLock(mock, 7, oauthIdentityNamespace{Protocol: "oauth2", AuthURL: "https://old.example/auth", TokenURL: "https://old.example/token", UserinfoURL: "https://old.example/user", SubjectField: "id", ClientID: "client"}, 2)
	mock.ExpectQuery("SELECT EXISTS").WithArgs(7).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery("UPDATE auth_provider SET").
		WithArgs("Provider", "icon", "oidc", "https://issuer.example", "", "", "", "openid profile", "sub", "client", "secret", false, true, int64(3), int64(10), 7).
		WillReturnRows(sqlmock.NewRows([]string{"identity_namespace_version", "config_revision"}).AddRow(3, 10))
	mock.ExpectExec("DELETE FROM auth_identity").WithArgs(7).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	version, revision, err := UpdateOAuthProvider(7, "Provider", "icon", "oidc", "https://issuer.example", "", "", "", "openid profile", "sub", "client", "secret", false, true)
	if err != nil {
		t.Fatal(err)
	}
	if version != 3 {
		t.Fatalf("version = %d, want 3", version)
	}
	if revision != 10 {
		t.Fatalf("revision = %d, want 10", revision)
	}
}

func TestUpdateOAuthProviderKeepsVersionForCosmeticChange(t *testing.T) {
	mock := setOAuthMockDB(t)
	current := oauthIdentityNamespace{Protocol: "oidc", IssuerURL: "https://issuer.example", SubjectField: "sub", ClientID: "client"}
	expectProviderLock(mock, 7, current, 5)
	mock.ExpectQuery("UPDATE auth_provider SET").
		WithArgs("Renamed", "new-icon", "oidc", "https://issuer.example", "", "", "", "openid email", "sub", "client", "new-secret", true, true, int64(5), int64(13), 7).
		WillReturnRows(sqlmock.NewRows([]string{"identity_namespace_version", "config_revision"}).AddRow(5, 13))
	mock.ExpectCommit()

	version, revision, err := UpdateOAuthProvider(7, "Renamed", "new-icon", "oidc", "https://issuer.example", "", "", "", "openid email", "sub", "client", "new-secret", true, true)
	if err != nil {
		t.Fatal(err)
	}
	if version != 5 {
		t.Fatalf("version = %d, want 5", version)
	}
	if revision != 13 {
		t.Fatalf("revision = %d, want 13", revision)
	}
}

func TestAddAuthIdentityRejectsStaleNamespaceVersion(t *testing.T) {
	mock := setOAuthMockDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT identity_namespace_version, config_revision, is_enabled FROM auth_provider").
		WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"identity_namespace_version", "config_revision", "is_enabled"}).AddRow(6, 13, true))
	mock.ExpectRollback()

	_, err := AddAuthIdentity(42, 7, 5, 12, "subject-a", "a@example.com", "A")
	if !errors.Is(err, ErrOAuthIdentityNamespaceChanged) {
		t.Fatalf("AddAuthIdentity() error = %v, want ErrOAuthIdentityNamespaceChanged", err)
	}
}

func TestAddAuthIdentityMapsBindingUniqueViolations(t *testing.T) {
	for _, constraint := range []string{"U_OAUTH", "auth_identity_active_user_provider_unique"} {
		t.Run(constraint, func(t *testing.T) {
			mock := setOAuthMockDB(t)
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT identity_namespace_version, config_revision, is_enabled FROM auth_provider").
				WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"identity_namespace_version", "config_revision", "is_enabled"}).AddRow(5, 12, true))
			mock.ExpectQuery("INSERT INTO auth_identity").
				WithArgs(int64(42), 7, "subject-a", "a@example.com", "A").
				WillReturnError(&pq.Error{Code: "23505", Constraint: constraint})
			mock.ExpectRollback()

			_, err := AddAuthIdentity(42, 7, 5, 12, "subject-a", "a@example.com", "A")
			if !errors.Is(err, ErrOAuthIdentityAlreadyLinked) {
				t.Fatalf("AddAuthIdentity() error = %v, want ErrOAuthIdentityAlreadyLinked", err)
			}
		})
	}
}

func TestAddAuthIdentityThenBlocksNamespaceChange(t *testing.T) {
	mock := setOAuthMockDB(t)
	query := regexp.QuoteMeta(`INSERT INTO auth_identity (user_id, provider_id, subject, email, name) 
		VALUES ($1, $2, $3, $4, $5) RETURNING id`)
	expectIdentityInsert(mock, query, 42, 7, 2, "subject-a", "a@example.com", "A", 101)
	expectProviderLock(mock, 7, oauthIdentityNamespace{Protocol: "oauth2", AuthURL: "https://old.example/auth", TokenURL: "https://old.example/token", UserinfoURL: "https://old.example/user", SubjectField: "id", ClientID: "client"}, 2)
	mock.ExpectQuery("SELECT EXISTS").WithArgs(7).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectRollback()

	if _, err := AddAuthIdentity(42, 7, 2, 9, "subject-a", "a@example.com", "A"); err != nil {
		t.Fatal(err)
	}
	_, _, err := UpdateOAuthProvider(7, "Provider", "icon", "oidc", "https://issuer.example", "", "", "", "openid", "sub", "client", "secret", false, true)
	if !errors.Is(err, ErrOAuthIdentityNamespaceLocked) {
		t.Fatalf("UpdateOAuthProvider() error = %v, want ErrOAuthIdentityNamespaceLocked", err)
	}
}

func TestGetAuthIdentityBySubjectRejectsStaleNamespaceVersion(t *testing.T) {
	mock := setOAuthMockDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT identity_namespace_version, config_revision, is_enabled FROM auth_provider").
		WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"identity_namespace_version", "config_revision", "is_enabled"}).AddRow(6, 13, true))
	mock.ExpectRollback()

	_, err := GetAuthIdentityBySubject(7, 5, 12, "subject-a")
	if !errors.Is(err, ErrOAuthIdentityNamespaceChanged) {
		t.Fatalf("GetAuthIdentityBySubject() error = %v, want ErrOAuthIdentityNamespaceChanged", err)
	}
}

func TestGetAuthIdentityBySubjectRejectsDisabledProvider(t *testing.T) {
	mock := setOAuthMockDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT identity_namespace_version, config_revision, is_enabled FROM auth_provider").
		WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"identity_namespace_version", "config_revision", "is_enabled"}).AddRow(5, 12, false))
	mock.ExpectRollback()

	_, err := GetAuthIdentityBySubject(7, 5, 12, "subject-a")
	if !errors.Is(err, ErrOAuthIdentityNamespaceChanged) {
		t.Fatalf("GetAuthIdentityBySubject() error = %v, want ErrOAuthIdentityNamespaceChanged", err)
	}
}

func expectProviderLock(mock sqlmock.Sqlmock, providerID int, namespace oauthIdentityNamespace, version int64) {
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT protocol, issuer_url, auth_url, token_url, userinfo_url, subject_field, client_id, identity_namespace_version, config_revision").
		WithArgs(providerID).
		WillReturnRows(sqlmock.NewRows([]string{"protocol", "issuer_url", "auth_url", "token_url", "userinfo_url", "subject_field", "client_id", "identity_namespace_version", "config_revision"}).
			AddRow(namespace.Protocol, namespace.IssuerURL, namespace.AuthURL, namespace.TokenURL, namespace.UserinfoURL, namespace.SubjectField, namespace.ClientID, version, version+7))
}

func expectIdentityInsert(mock sqlmock.Sqlmock, query string, userID int64, providerID int, version int64, subject, email, name string, identityID int64) {
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT identity_namespace_version, config_revision, is_enabled FROM auth_provider").
		WithArgs(providerID).WillReturnRows(sqlmock.NewRows([]string{"identity_namespace_version", "config_revision", "is_enabled"}).AddRow(version, version+7, true))
	mock.ExpectQuery(query).WithArgs(userID, providerID, subject, email, name).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(identityID))
	mock.ExpectCommit()
}

func setOAuthMockDB(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	oldDB := Db
	Db = sqlx.NewDb(database, "sqlmock")
	t.Cleanup(func() {
		Db = oldDB
		_ = database.Close()
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet SQL expectations: %v", err)
		}
	})
	return mock
}
