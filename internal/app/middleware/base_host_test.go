package middleware

import "testing"

func TestHealthPathsBypassConfiguredHostRestriction(t *testing.T) {
	for _, path := range []string{"/health", "/health/live", "/health/ready"} {
		if !isHealthPath(path) {
			t.Errorf("health path %q was not recognized", path)
		}
	}
	for _, path := range []string{"/", "/healthcheck", "/health/ready/more"} {
		if isHealthPath(path) {
			t.Errorf("non-health path %q was recognized", path)
		}
	}
}
