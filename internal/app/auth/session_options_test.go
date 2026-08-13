package auth

import (
	"mosona-manager/internal/config"
	"testing"
)

func TestSessionOptionsUseValidatedSecureCookiesSetting(t *testing.T) {
	previous := config.Conf.SecureCookies
	t.Cleanup(func() { config.Conf.SecureCookies = previous })

	config.Conf.SecureCookies = true
	if options := sessionOptions(3600); !options.Secure {
		t.Fatal("sessionOptions() Secure = false, want true")
	}

	config.Conf.SecureCookies = false
	if options := sessionOptions(3600); options.Secure {
		t.Fatal("sessionOptions() Secure = true, want false")
	}
}
