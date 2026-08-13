package apublic

import (
	"testing"
	"time"
)

func TestPublicSSELimiterEnforcesAndReleasesLimits(t *testing.T) {
	limiter := newPublicSSELimiter(3, 2, 1)
	releaseA, ok := limiter.acquire(7, "192.0.2.1")
	if !ok {
		t.Fatal("first connection was rejected")
	}
	if _, ok := limiter.acquire(8, "192.0.2.1"); ok {
		t.Fatal("per-IP connection limit was not enforced")
	}
	releaseB, ok := limiter.acquire(7, "192.0.2.2")
	if !ok {
		t.Fatal("second team connection was rejected")
	}
	if _, ok := limiter.acquire(7, "192.0.2.3"); ok {
		t.Fatal("per-team connection limit was not enforced")
	}
	releaseC, ok := limiter.acquire(8, "192.0.2.3")
	if !ok {
		t.Fatal("third global connection was rejected")
	}
	if _, ok := limiter.acquire(9, "192.0.2.4"); ok {
		t.Fatal("global connection limit was not enforced")
	}

	releaseA()
	releaseA()
	releaseD, ok := limiter.acquire(9, "192.0.2.1")
	if !ok {
		t.Fatal("released connection capacity was not reusable")
	}
	releaseB()
	releaseC()
	releaseD()
}

func TestPublicRequestLimiterEnforcesGlobalAndIPWindows(t *testing.T) {
	limiter := newPublicRequestLimiter(3, 2, time.Second)
	now := time.Unix(100, 0)
	if !limiter.allow(now, "192.0.2.1") || !limiter.allow(now, "192.0.2.1") {
		t.Fatal("requests within limits were rejected")
	}
	if limiter.allow(now, "192.0.2.1") {
		t.Fatal("per-IP request limit was not enforced")
	}
	if !limiter.allow(now, "192.0.2.2") {
		t.Fatal("third global request was rejected")
	}
	if limiter.allow(now, "192.0.2.3") {
		t.Fatal("global request limit was not enforced")
	}
	if !limiter.allow(now.Add(time.Second), "192.0.2.1") {
		t.Fatal("request window did not reset")
	}
}
