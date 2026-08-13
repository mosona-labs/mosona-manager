package auth

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gorilla/sessions"
	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo-contrib/v5/session"
	"github.com/labstack/echo/v5"
	"golang.org/x/oauth2"
	"mosona-manager/internal/db"
	mosonaoauth "mosona-manager/internal/oauth"
)

type callbackVerifier struct {
	subjects map[string]string
	nonce    string
}

type callbackRoundTripper func(*http.Request) (*http.Response, error)

func (fn callbackRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func (v *callbackVerifier) Verify(_ context.Context, raw string) (mosonaoauth.VerifiedOIDCIdentity, error) {
	return mosonaoauth.VerifiedOIDCIdentity{
		Subject: v.subjects[raw],
		Nonce:   v.nonce,
		Claims:  []byte(`{"email":"user@example.com","name":"User"}`),
	}, nil
}

func TestOIDCCallbackUsesDistinctVerifiedSubjectsAndConsumesState(t *testing.T) {
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	oldDB := db.Db
	db.Db = sqlx.NewDb(database, "sqlmock")
	t.Cleanup(func() {
		db.Db = oldDB
		_ = database.Close()
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet SQL expectations: %v", err)
		}
	})

	tokenClient := &http.Client{Transport: callbackRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://provider.example/token" || r.Method != http.MethodPost {
			t.Fatalf("unexpected token request: %s %s", r.Method, r.URL)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		body, err := json.Marshal(map[string]any{
			"access_token": "access-token",
			"token_type":   "Bearer",
			"id_token":     "signed-" + r.Form.Get("code"),
		})
		if err != nil {
			t.Fatal(err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader(body)),
			Request:    r,
		}, nil
	})}

	const providerID = 7
	verifier := &callbackVerifier{subjects: map[string]string{
		"signed-code-a": "subject-a",
		"signed-code-b": "subject-b",
	}}
	mosonaoauth.InstallProvider(providerID, &mosonaoauth.ProviderConfig{
		Config: &oauth2.Config{
			ClientID:     "client-id",
			ClientSecret: "client-secret",
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://provider.example/authorize",
				TokenURL: "https://provider.example/token",
			},
		},
		Protocol:                 mosonaoauth.ProtocolOIDC,
		OIDC:                     verifier,
		IdentityNamespaceVersion: 1,
		ConfigRevision:           1,
	})
	t.Cleanup(func() { mosonaoauth.RemoveProvider(providerID, 1<<63-1) })

	e := echo.New()
	e.Use(session.Middleware(sessions.NewCookieStore([]byte("01234567890123456789012345678901"))))
	Router(e.Group("/api/auth"))

	identityQuery := regexp.QuoteMeta(
		"SELECT id, user_id, provider_id, subject, email, name, quarantined FROM auth_identity WHERE provider_id=$1 AND subject=$2 AND quarantined=false AND subject<>'0'",
	)
	providerVersionQuery := "SELECT identity_namespace_version, config_revision, is_enabled FROM auth_provider"

	var firstCookie *http.Cookie
	var firstState string
	for _, tc := range []struct {
		code    string
		subject string
	}{
		{code: "code-a", subject: "subject-a"},
		{code: "code-b", subject: "subject-b"},
	} {
		login := httptest.NewRecorder()
		e.ServeHTTP(login, httptest.NewRequest(http.MethodGet, "/api/auth/oauth/7", nil))
		if login.Code != http.StatusOK {
			t.Fatalf("login start status = %d, body = %s", login.Code, login.Body.String())
		}
		cookies := login.Result().Cookies()
		if len(cookies) == 0 {
			t.Fatal("login start did not set a session cookie")
		}

		var response struct {
			Data struct {
				URL   string `json:"url"`
				State string `json:"state"`
			} `json:"data"`
		}
		if err := json.Unmarshal(login.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		authURL, err := url.Parse(response.Data.URL)
		if err != nil {
			t.Fatal(err)
		}
		verifier.nonce = authURL.Query().Get("nonce")
		if verifier.nonce == "" || authURL.Query().Get("state") != response.Data.State {
			t.Fatalf("authorization URL is missing bound state/nonce: %s", response.Data.URL)
		}

		mock.ExpectBegin()
		mock.ExpectQuery(providerVersionQuery).
			WithArgs(providerID).
			WillReturnRows(sqlmock.NewRows([]string{"identity_namespace_version", "config_revision", "is_enabled"}).AddRow(1, 1, true))
		mock.ExpectQuery(identityQuery).
			WithArgs(providerID, tc.subject).
			WillReturnError(sql.ErrNoRows)
		mock.ExpectRollback()

		form := url.Values{"code": {tc.code}, "state": {response.Data.State}}
		callbackRequest := httptest.NewRequest(
			http.MethodPost,
			"/api/auth/oauth/7",
			strings.NewReader(form.Encode()),
		)
		callbackRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		callbackRequest.AddCookie(cookies[0])
		callbackRequest = callbackRequest.WithContext(
			context.WithValue(callbackRequest.Context(), oauth2.HTTPClient, tokenClient),
		)
		callback := httptest.NewRecorder()
		e.ServeHTTP(callback, callbackRequest)
		if callback.Code != http.StatusNotFound || !strings.Contains(callback.Body.String(), `"code":"not_found"`) {
			t.Fatalf("callback status = %d, body = %s", callback.Code, callback.Body.String())
		}

		if firstCookie == nil {
			firstCookie = cookies[0]
			firstState = response.Data.State
		}
	}

	replayForm := url.Values{"code": {"code-a"}, "state": {firstState}}
	replayRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/auth/oauth/7",
		strings.NewReader(replayForm.Encode()),
	)
	replayRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	replayRequest.AddCookie(firstCookie)
	replay := httptest.NewRecorder()
	e.ServeHTTP(replay, replayRequest)
	if replay.Code != http.StatusBadRequest || !strings.Contains(replay.Body.String(), "Invalid or expired OAuth state") {
		t.Fatalf("replayed callback status = %d, body = %s", replay.Code, replay.Body.String())
	}
}

func TestOIDCCallbackRejectsStateFromPreviousIdentityNamespace(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	oldDB := db.Db
	db.Db = sqlx.NewDb(database, "sqlmock")
	t.Cleanup(func() {
		db.Db = oldDB
		_ = database.Close()
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet SQL expectations: %v", err)
		}
	})

	const providerID = 8
	provider := &mosonaoauth.ProviderConfig{
		Config: &oauth2.Config{Endpoint: oauth2.Endpoint{
			AuthURL: "https://provider.example/authorize", TokenURL: "https://provider.example/token",
		}},
		Protocol:                 mosonaoauth.ProtocolOIDC,
		IdentityNamespaceVersion: 1,
		ConfigRevision:           1,
	}
	mosonaoauth.InstallProvider(providerID, provider)
	t.Cleanup(func() { mosonaoauth.RemoveProvider(providerID, 1<<63-1) })

	e := echo.New()
	e.Use(session.Middleware(sessions.NewCookieStore([]byte("01234567890123456789012345678901"))))
	Router(e.Group("/api/auth"))
	login := httptest.NewRecorder()
	e.ServeHTTP(login, httptest.NewRequest(http.MethodGet, "/api/auth/oauth/8", nil))
	var response struct {
		Data struct {
			State string `json:"state"`
		} `json:"data"`
	}
	if err = json.Unmarshal(login.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	mosonaoauth.InstallProvider(providerID, &mosonaoauth.ProviderConfig{
		Config: &oauth2.Config{Endpoint: oauth2.Endpoint{
			AuthURL: "https://provider.example/authorize", TokenURL: "https://provider.example/token",
		}},
		Protocol:                 mosonaoauth.ProtocolOIDC,
		IdentityNamespaceVersion: 2,
		ConfigRevision:           2,
	})

	form := url.Values{"code": {"unused"}, "state": {response.Data.State}}
	request := httptest.NewRequest(http.MethodPost, "/api/auth/oauth/8", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(login.Result().Cookies()[0])
	callback := httptest.NewRecorder()
	e.ServeHTTP(callback, request)
	if callback.Code != http.StatusBadRequest || !strings.Contains(callback.Body.String(), "Invalid or expired OAuth state") {
		t.Fatalf("callback status = %d, body = %s", callback.Code, callback.Body.String())
	}
}

func TestDisabledOAuthProviderRejectsDirectURLAndPendingState(t *testing.T) {
	const providerID = 9
	provider := &mosonaoauth.ProviderConfig{
		Config: &oauth2.Config{
			ClientID: "client-id",
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://provider.example/authorize",
				TokenURL: "https://provider.example/token",
			},
		},
		Protocol:                 mosonaoauth.ProtocolOAuth2,
		IdentityNamespaceVersion: 1,
		ConfigRevision:           1,
	}
	mosonaoauth.InstallProvider(providerID, provider)
	t.Cleanup(func() { mosonaoauth.RemoveProvider(providerID, 1<<63-1) })

	e := echo.New()
	e.Use(session.Middleware(sessions.NewCookieStore([]byte("01234567890123456789012345678901"))))
	Router(e.Group("/api/auth"))

	login := httptest.NewRecorder()
	e.ServeHTTP(login, httptest.NewRequest(http.MethodGet, "/api/auth/oauth/9", nil))
	if login.Code != http.StatusOK {
		t.Fatalf("login start status = %d, body = %s", login.Code, login.Body.String())
	}
	var response struct {
		Data struct {
			State string `json:"state"`
		} `json:"data"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.State == "" || len(login.Result().Cookies()) == 0 {
		t.Fatal("login start did not persist OAuth state")
	}

	mosonaoauth.RemoveProvider(providerID, 1)
	direct := httptest.NewRecorder()
	e.ServeHTTP(direct, httptest.NewRequest(http.MethodGet, "/api/auth/oauth/9", nil))
	if direct.Code != http.StatusBadRequest || !strings.Contains(direct.Body.String(), "Invalid OAuth provider") {
		t.Fatalf("disabled direct URL status = %d, body = %s", direct.Code, direct.Body.String())
	}

	provider.ConfigRevision = 2
	mosonaoauth.InstallProvider(providerID, provider)
	tokenRequests := 0
	tokenClient := &http.Client{Transport: callbackRoundTripper(func(request *http.Request) (*http.Response, error) {
		tokenRequests++
		return nil, errors.New("token endpoint must not be called for revoked state")
	})}
	form := url.Values{"code": {"unused"}, "state": {response.Data.State}}
	request := httptest.NewRequest(http.MethodPost, "/api/auth/oauth/9", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(login.Result().Cookies()[0])
	request = request.WithContext(context.WithValue(request.Context(), oauth2.HTTPClient, tokenClient))
	callback := httptest.NewRecorder()
	e.ServeHTTP(callback, request)

	if callback.Code != http.StatusBadRequest || !strings.Contains(callback.Body.String(), "Invalid or expired OAuth state") {
		t.Fatalf("revoked callback status = %d, body = %s", callback.Code, callback.Body.String())
	}
	if tokenRequests != 0 {
		t.Fatalf("revoked callback made %d token requests", tokenRequests)
	}
}
