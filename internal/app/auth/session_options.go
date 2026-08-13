package auth

import (
	"mosona-manager/internal/config"
	"net/http"

	"github.com/gorilla/sessions"
)

func sessionOptions(maxAge int) *sessions.Options {
	return &sessions.Options{
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   config.Conf.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	}
}

func StoreOptions(maxAge int) *sessions.Options {
	return sessionOptions(maxAge)
}
