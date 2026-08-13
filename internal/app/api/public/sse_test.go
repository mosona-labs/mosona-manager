package apublic

import (
	"fmt"
	"testing"
	"time"
)

func TestPublicSSELimiterEnforcesAndReleasesIPLimit(t *testing.T) {
	limiter := newPublicSSELimiter(2)
	releaseA, ok := limiter.acquire("192.0.2.1")
	if !ok {
		t.Fatal("first connection was rejected")
	}
	releaseB, ok := limiter.acquire("192.0.2.1")
	if !ok {
		t.Fatal("second connection was rejected")
	}
	if _, ok := limiter.acquire("192.0.2.1"); ok {
		t.Fatal("per-IP connection limit was not enforced")
	}

	releaseA()
	releaseA()
	releaseC, ok := limiter.acquire("192.0.2.1")
	if !ok {
		t.Fatal("released connection capacity was not reusable")
	}
	releaseB()
	releaseC()
}

func TestPublicSSELimiterHasNoGlobalOrTeamLimit(t *testing.T) {
	limiter := newPublicSSELimiter(publicSSEIPLimit)
	const connections = 10_000
	releases := make([]func(), 0, connections)
	for i := range connections {
		release, ok := limiter.acquire(fmt.Sprintf("192.0.2.%d", i))
		if !ok {
			t.Fatalf("connection %d of %d was rejected", i+1, connections)
		}
		releases = append(releases, release)
	}
	for _, release := range releases {
		release()
	}
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
