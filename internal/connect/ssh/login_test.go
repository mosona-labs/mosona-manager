package ssh

import (
	"strings"
	"testing"
	"time"
)

func TestParseLoginProbe(t *testing.T) {
	output := strings.Join([]string{
		"self_ip=198.51.100.7",
		"login|root|pts/0|203.0.113.7|Fri Oct 8 12:00:23 2026",
		"login|deploy|pts/1|2001:db8::1|Fri Oct 8 11:00:00 2026",
		"login||pts/2|192.0.2.1|Fri Oct 8 10:00:00 2026",
		"login|admin|tty1|console-only|Fri Oct 8 09:00:00 2026",
		"login|bad|pts/3|10.0.0.1",
		"",
	}, "\n")

	probe := parseLoginProbe(output)

	if probe.selfIP != "198.51.100.7" {
		t.Fatalf("selfIP = %q", probe.selfIP)
	}
	if probe.unavailable {
		t.Fatal("probe marked unavailable")
	}
	if len(probe.sessions) != 2 {
		t.Fatalf("sessions = %#v, want 2 valid entries", probe.sessions)
	}
	first := probe.sessions[0]
	if first.user != "root" || first.tty != "pts/0" || first.ip != "203.0.113.7" || first.date != "Fri Oct 8 12:00:23 2026" {
		t.Fatalf("first session = %#v", first)
	}
	if probe.sessions[1].ip != "2001:db8::1" {
		t.Fatalf("ipv6 session = %#v", probe.sessions[1])
	}
}

func TestParseLoginProbeUnavailable(t *testing.T) {
	probe := parseLoginProbe("self_ip=198.51.100.7\nlogins_unavailable=1\n")
	if !probe.unavailable {
		t.Fatal("probe not marked unavailable")
	}
	if probe.selfIP != "198.51.100.7" {
		t.Fatalf("selfIP = %q", probe.selfIP)
	}
}

func TestDiffLoginSessionsExcludesSeenAndMonitorSelf(t *testing.T) {
	prev := map[string]struct{}{
		"root|pts/0|203.0.113.7|2026-10-08T12:00:23Z": {},
	}
	current := []loginSession{
		{user: "root", tty: "pts/0", ip: "203.0.113.7", date: "Fri Oct 8 12:00:23 2026"},
		{user: "root", tty: "pts/9", ip: "198.51.100.7", date: "Fri Oct 8 12:01:00 2026"},
		{user: "admin", tty: "pts/2", ip: "192.0.2.5", date: "Fri Oct 8 12:02:00 2026"},
	}

	fresh, snapshot := diffLoginSessions(prev, current, "root", "198.51.100.7")

	if len(fresh) != 1 || fresh[0].user != "admin" {
		t.Fatalf("fresh = %#v, want only the admin session", fresh)
	}
	if len(snapshot) != 3 {
		t.Fatalf("snapshot size = %d, want 3", len(snapshot))
	}
}

func TestLoginSessionKeysAcrossReaders(t *testing.T) {
	const prefix = "login|root|pts/0|192.0.2.1|"
	for _, stamp := range []string{
		"2026-10-08T12:00:23,123456+00:00",
		"2026-10-08T20:00:23.123456+08:00",
		"Thu Oct 8 12:00:23 2026",
		"Thu Oct 8 12:00", // old last, without -F
	} {
		t.Run(stamp, func(t *testing.T) {
			fast := parseLoginProbe(prefix + "2026-10-08T12:00:23,654321+00:00\n")
			fallback := parseLoginProbe(prefix + stamp + "\n")
			var snapshot map[string]struct{}
			for _, probe := range []loginProbe{fast, fallback, fast, fallback} {
				fresh, next := diffLoginSessions(snapshot, probe.sessions, "monitor", "")
				if len(fresh) != 0 {
					t.Fatalf("same login was reported again: %#v", fresh)
				}
				snapshot = next
			}
		})
	}
}

func TestLoginSessionKeysKeepSecondPrecision(t *testing.T) {
	baseline := parseLoginProbe("login|root|pts/0|192.0.2.1|2026-10-08T12:00:23,123456+00:00\n")
	_, snapshot := diffLoginSessions(nil, baseline.sessions, "monitor", "")
	current := parseLoginProbe("login|root|pts/0|192.0.2.1|Thu Oct 8 12:00:24 2026\n")
	fresh, _ := diffLoginSessions(snapshot, current.sessions, "monitor", "")
	if len(fresh) != 1 {
		t.Fatalf("different login in the same minute was lost: %#v", fresh)
	}
}

func TestParseLoginProbeValidatesAndCanonicalizesIP(t *testing.T) {
	probe := parseLoginProbe(strings.Join([]string{
		"self_ip=2001:0db8:0:0:0:0:0:1",
		"login|root|pts/0|2001:db8::1|old",
		"login|root|pts/1|gateway.example.org|old",
		"login|root|pts/2|999.999.999.999|old",
		"login|root|pts/3|::::|old",
		"login|root|pts/4|0.0.0.0|old",
		"login|root|pts/5|::|old",
	}, "\n"))
	if len(probe.sessions) != 1 || !probe.sessions[0].isMonitorSelf("root", probe.selfIP) {
		t.Fatalf("invalid addresses accepted or self address not canonicalized: %#v", probe)
	}
}

func TestDiffLoginSessionsFirstPollSeedsBaseline(t *testing.T) {
	current := []loginSession{
		{user: "admin", tty: "pts/2", ip: "192.0.2.5", date: "Fri Oct 8 12:02:00 2026"},
	}

	fresh, snapshot := diffLoginSessions(nil, current, "root", "")

	if len(fresh) != 0 {
		t.Fatalf("fresh = %#v, want none on baseline poll", fresh)
	}
	if len(snapshot) != 1 {
		t.Fatalf("snapshot size = %d, want 1", len(snapshot))
	}
}

func TestLoginProberSinceEpoch(t *testing.T) {
	prober := &loginProber{}
	if got := prober.sinceEpoch(); got != 0 {
		t.Fatalf("baseline sinceEpoch = %d, want 0", got)
	}

	prober.lastPollAt = time.Now()
	want := prober.lastPollAt.Add(-loginSinceOverlap).Unix()
	if got := prober.sinceEpoch(); got != want {
		t.Fatalf("incremental sinceEpoch = %d, want %d", got, want)
	}
}
