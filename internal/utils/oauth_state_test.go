package utils

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/v5/session"
	"github.com/labstack/echo/v5"
)

func TestOAuthStateConsumesNonceOnce(t *testing.T) {
	store := sessions.NewCookieStore([]byte("01234567890123456789012345678901"))
	e := echo.New()
	e.Use(session.Middleware(store))
	e.GET("/save", func(c *echo.Context) error {
		if err := SaveOAuthState(c, 7, "state", "nonce", 9, 12); err != nil {
			return err
		}
		return c.NoContent(http.StatusNoContent)
	})
	e.GET("/consume", func(c *echo.Context) error {
		authorizationState, ok, err := ConsumeOAuthState(c, 7, c.QueryParam("state"))
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, map[string]any{"nonce": authorizationState.Nonce, "ok": ok, "revision": authorizationState.ConfigRevision, "version": authorizationState.IdentityNamespaceVersion})
	})

	save := httptest.NewRecorder()
	e.ServeHTTP(save, httptest.NewRequest(http.MethodGet, "/save", nil))
	cookie := save.Result().Cookies()[0]

	bad := httptest.NewRequest(http.MethodGet, "/consume?state=wrong", nil)
	bad.AddCookie(cookie)
	badResult := httptest.NewRecorder()
	e.ServeHTTP(badResult, bad)
	if badResult.Body.String() != `{"nonce":"","ok":false,"revision":0,"version":0}`+"\n" {
		t.Fatalf("wrong state response = %s", badResult.Body.String())
	}

	valid := httptest.NewRequest(http.MethodGet, "/consume?state=state", nil)
	valid.AddCookie(cookie)
	validResult := httptest.NewRecorder()
	e.ServeHTTP(validResult, valid)
	if validResult.Body.String() != `{"nonce":"nonce","ok":true,"revision":12,"version":9}`+"\n" {
		t.Fatalf("valid state response = %s", validResult.Body.String())
	}
	consumedCookie := validResult.Result().Cookies()[0]

	reused := httptest.NewRequest(http.MethodGet, "/consume?state=state", nil)
	reused.AddCookie(consumedCookie)
	reusedResult := httptest.NewRecorder()
	e.ServeHTTP(reusedResult, reused)
	if reusedResult.Body.String() != `{"nonce":"","ok":false,"revision":0,"version":0}`+"\n" {
		t.Fatalf("reused state response = %s", reusedResult.Body.String())
	}
}
