package auth

import (
	"net/http"

	"github.com/gorilla/sessions"
)

func sessionOptions(maxAge int) *sessions.Options {
	return &sessions.Options{
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}
