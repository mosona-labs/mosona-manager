package siteaccess

import "testing"

func TestRefreshExcludesBaseHostFromPublicHosts(t *testing.T) {
	mu.Lock()
	snap = snapshot{
		baseHost: "hub.example.com",
		publicHosts: map[string]struct{}{
			"hub.example.com":    {},
			"status.example.com": {},
		},
	}
	mu.Unlock()

	if IsPublicPageHost("hub.example.com") {
		t.Fatal("base host must not be treated as public page host")
	}
	if !IsPublicPageHost("status.example.com") {
		t.Fatal("other public host should match")
	}
}

func TestPublicPagePathAllowed(t *testing.T) {
	allowed := []string{
		"/",
		"/health",
		"/api/public/preview/bootstrap",
		"/preview-assets/app.js",
		"/flags/nl.svg",
		"/icons/macos.svg",
		"/avatars/uuid.avif",
	}
	for _, p := range allowed {
		if !PublicPagePathAllowed(p) {
			t.Fatalf("expected allowed: %s", p)
		}
	}
	denied := []string{
		"/api/agent/enroll",
		"/api/v1/user/me",
		"/login",
		"/static/app.js",
	}
	for _, p := range denied {
		if PublicPagePathAllowed(p) {
			t.Fatalf("expected denied: %s", p)
		}
	}
}
