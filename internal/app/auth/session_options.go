package auth

import (
	"net/http"
	"os"

	"github.com/gorilla/sessions"
)

func sessionOptions(maxAge int) *sessions.Options {
	return &sessions.Options{
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   os.Getenv("SECURE_COOKIES") == "true",
		SameSite: http.SameSiteLaxMode,
	}
}

func StoreOptions(maxAge int) *sessions.Options {
	return sessionOptions(maxAge)
}
