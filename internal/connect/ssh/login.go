package ssh

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"mosona-manager/internal/connect/ssh/script"
	"mosona-manager/internal/loginnotify"

	gossh "golang.org/x/crypto/ssh"
)

const (
	loginProbeInterval = 30 * time.Second
	loginProbeTimeout  = 15 * time.Second
	loginRetryMaxDelay = 5 * time.Minute
	// Re-request a small window on the server clock to cover records written
	// around the previous probe; session keys discard repeated entries.
	loginSinceOverlap = 5 * time.Minute
	// Re-read a bounded number of wtmp records on each probe so entries
	// appended around the previous poll are covered; session keys discard
	// repeated entries.
	loginWtmpOverlap = 128
	loginLastLines   = 50
)

// loginSession is a single login entry reported by linux_logins.sh.
type loginSession struct {
	user string
	tty  string
	ip   string
	date string
}

func (s loginSession) key() string {
	stamp := s.date
	if started, ok := loginTime(stamp); ok {
		stamp = started.UTC().Format(time.RFC3339)
	}
	return s.user + "|" + s.tty + "|" + s.ip + "|" + stamp
}

func loginTime(stamp string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, "Mon Jan 2 15:04:05 2006"} {
		if started, err := time.Parse(layout, stamp); err == nil {
			return started, true
		}
	}
	return time.Time{}, false
}

// Old last implementations omit the year and seconds. Match their reduced
// precision only when one side of the comparison actually uses that format.
func loginMinuteKey(key string) (string, bool) {
	index := strings.LastIndexByte(key, '|') + 1
	if index == 0 {
		return "", false
	}
	stamp := key[index:]
	if started, ok := loginTime(stamp); ok {
		return key[:index] + started.UTC().Format("Mon Jan 2 15:04"), true
	}
	if _, err := time.Parse("Mon Jan 2 15:04", stamp); err == nil {
		return key, false
	}
	return "", false
}

// isMonitorSelf reports whether the session matches Mosona's own monitoring
// connection (same user coming from the manager egress IP as seen by sshd).
// Mosona's exec sessions do not allocate a PTY and are not recorded in wtmp,
// so this is a defense-in-depth filter on top of that.
func (s loginSession) isMonitorSelf(monitorUser, selfIP string) bool {
	return selfIP != "" && s.ip == selfIP && s.user == monitorUser
}

// loginProbe is the parsed output of one linux_logins.sh run.
type loginProbe struct {
	selfIP      string
	serverEpoch int64
	// wtmpRecords is the server's wtmp record count when the probe used the
	// incremental read path; it becomes the cursor for the next probe.
	wtmpRecords int64
	unavailable bool
	sessions    []loginSession
}

func parseLoginProbe(output string) loginProbe {
	probe := loginProbe{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "self_ip="):
			probe.selfIP = strings.TrimSpace(strings.TrimPrefix(line, "self_ip="))
			if ip := net.ParseIP(probe.selfIP); ip != nil {
				probe.selfIP = ip.String()
			}
		case strings.HasPrefix(line, "server_epoch="):
			probe.serverEpoch, _ = strconv.ParseInt(strings.TrimPrefix(line, "server_epoch="), 10, 64)
		case strings.HasPrefix(line, "wtmp_records="):
			probe.wtmpRecords, _ = strconv.ParseInt(strings.TrimPrefix(line, "wtmp_records="), 10, 64)
		case line == "logins_unavailable=1":
			probe.unavailable = true
		case strings.HasPrefix(line, "login|"):
			parts := strings.SplitN(line, "|", 5)
			if len(parts) != 5 {
				continue
			}
			session := loginSession{
				user: parts[1],
				tty:  parts[2],
				ip:   parts[3],
				date: parts[4],
			}
			if session.user == "" || session.ip == "" {
				continue
			}
			ip := net.ParseIP(session.ip)
			if ip == nil || ip.IsUnspecified() {
				continue
			}
			session.ip = ip.String()
			probe.sessions = append(probe.sessions, session)
		}
	}
	return probe
}

// loginProber keeps the script in a shell process for this SSH connection.
// Only numeric poll arguments cross stdin after startup; no script cache is
// written to the server's filesystem. Calls to probe are sequential.
type loginProber struct {
	client     *gossh.Client
	session    *gossh.Session
	stdin      io.WriteCloser
	stdout     *bufio.Reader
	lastPollAt time.Time // server clock, never the manager clock
	// wtmpRecords is the record cursor for the script's incremental wtmp
	// read; zero asks for the bounded tail window.
	wtmpRecords int64
	stage       string // accessed only by runProbe, then by probe after joining it
}

const loginProbeEnd = "mosona_login_probe_end="
const loginProbeStage = "login_probe_stage="

func loginProbeCommand(scriptFile string) string {
	// Each invocation runs in a subshell so the probe's exit/traps and shell
	// options do not terminate or modify the persistent request loop.
	program := "login_probe() {\n" + scriptFile + "\n}\n" + `export LOGIN_PROBE_PROGRESS=1
while read -r records epoch lines; do
  [ "$records" = 0 ] && records=''
  [ "$epoch" = 0 ] && epoch=''
  ( login_probe "$records" "$epoch" "$lines" ) </dev/null
  result=$?
  printf 'mosona_login_probe_end=%s\n' "$result"
done`
	return "sh -c '" + strings.ReplaceAll(program, "'", "'\"'\"'") + "'"
}

func (p *loginProber) start() error {
	scriptFile, err := script.GetScript("linux_logins.sh")
	if err != nil {
		return err
	}
	p.stage = "opening SSH session"
	session, err := p.client.NewSession()
	if err != nil {
		return err
	}
	p.session = session
	p.stdin, err = session.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}
	p.stdout = bufio.NewReader(stdout)
	session.Stderr = io.Discard
	p.stage = "starting remote shell"
	return session.Start(loginProbeCommand(scriptFile))
}

func (p *loginProber) close() {
	if p.session != nil {
		if p.stdin != nil {
			_ = p.stdin.Close()
		}
		_ = p.session.Close()
		p.session = nil
		p.stdin = nil
		p.stdout = nil
	}
}

// sinceEpoch returns the lower bound for the next incremental window, or 0
// for the baseline poll (full bounded window). Only the `last` fallback uses
// it; the incremental wtmp read is cursor-based.
func (p *loginProber) sinceEpoch() int64 {
	if p.lastPollAt.IsZero() {
		return 0
	}
	return p.lastPollAt.Add(-loginSinceOverlap).Unix()
}

// sinceRecords returns the wtmp record cursor for the next probe with a
// small overlap, or 0 for the baseline poll (bounded tail window).
func (p *loginProber) sinceRecords() int64 {
	if p.wtmpRecords <= loginWtmpOverlap {
		return 0
	}
	return p.wtmpRecords - loginWtmpOverlap
}

// The deadline covers starting the shell, sending arguments and reading a
// complete response. The prober owns a dedicated connection, so closing it
// unblocks SSH operations without interrupting status collection.
func (p *loginProber) probe(ctx context.Context) (loginProbe, error) {
	ctx, cancel := context.WithTimeout(ctx, loginProbeTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return loginProbe{}, err
	}
	type result struct {
		probe loginProbe
		err   error
	}
	done := make(chan result, 1)
	go func() {
		probe, err := p.runProbe()
		done <- result{probe, err}
	}()
	var r result
	select {
	case <-ctx.Done():
		_ = p.client.Close()
		// Join before touching prober fields or starting another probe.
		r = <-done
		r.err = ctx.Err()
	case r = <-done:
		if ctx.Err() != nil {
			r.err = ctx.Err()
		}
	}
	if r.err != nil {
		// Close the transport before closing the channel: a channel close can
		// itself block when the peer stops reading the shared SSH transport.
		_ = p.client.Close()
		p.close()
		return loginProbe{}, fmt.Errorf("%s: %w", p.stage, r.err)
	}
	// If the target cannot report its clock, retain bounded full snapshots
	// instead of substituting a potentially skewed manager clock.
	p.lastPollAt = time.Time{}
	if r.probe.serverEpoch > 0 {
		p.lastPollAt = time.Unix(r.probe.serverEpoch, 0)
	}
	// A reported cursor means the script read wtmp incrementally; retain the
	// previous cursor when the probe fell back to `last`.
	if r.probe.wtmpRecords > 0 {
		p.wtmpRecords = r.probe.wtmpRecords
	}
	return r.probe, nil
}

func (p *loginProber) runProbe() (loginProbe, error) {
	if p.session == nil {
		if err := p.start(); err != nil {
			return loginProbe{}, err
		}
	}
	p.stage = "sending probe arguments"
	if _, err := fmt.Fprintf(p.stdin, "%d %d %d\n", p.sinceRecords(), p.sinceEpoch(), loginLastLines); err != nil {
		return loginProbe{}, err
	}
	var output strings.Builder
	p.stage = "waiting for remote probe output"
	for {
		line, err := p.stdout.ReadString('\n')
		if err != nil {
			return loginProbe{}, fmt.Errorf("read login probe: %w", err)
		}
		if strings.HasPrefix(line, loginProbeStage) {
			// Accept only known stages so remote output cannot inject log text.
			switch strings.TrimSpace(strings.TrimPrefix(line, loginProbeStage)) {
			case "clock":
				p.stage = "reading remote clock"
			case "tempdir":
				p.stage = "creating remote temporary directory"
			case "wtmp":
				p.stage = "reading wtmp records"
			case "last":
				p.stage = "reading login history (last)"
			case "parse":
				p.stage = "parsing login history"
			case "cleanup":
				p.stage = "cleaning up remote probe"
			}
			continue
		}
		if strings.HasPrefix(line, loginProbeEnd) {
			status, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, loginProbeEnd)))
			if err == nil && status == 124 {
				return loginProbe{}, fmt.Errorf("remote command timeout: %w", context.DeadlineExceeded)
			}
			if err != nil || status != 0 {
				return loginProbe{}, fmt.Errorf("login probe failed: %s", strings.TrimSpace(line))
			}
			return parseLoginProbe(output.String()), nil
		}
		output.WriteString(line)
	}
}

// diffLoginSessions returns sessions in current that were not present in the
// previous snapshot (excluding Mosona's own monitoring connection) plus the
// new snapshot. The snapshot is replaced (not merged) because `last` output
// only ever loses entries from its tail, which keeps the state bounded.
func diffLoginSessions(prev map[string]struct{}, current []loginSession, monitorUser, selfIP string) (fresh []loginSession, snapshot map[string]struct{}) {
	snapshot = make(map[string]struct{}, len(current))
	previousMinutes := make(map[string]struct{}, len(prev))
	for key := range prev {
		if minute, _ := loginMinuteKey(key); minute != "" {
			previousMinutes[minute] = struct{}{}
		}
	}
	for _, session := range current {
		key := session.key()
		snapshot[key] = struct{}{}
		if prev != nil {
			_, seen := prev[key]
			if minute, precise := loginMinuteKey(key); !seen && minute != "" {
				if precise {
					_, seen = prev[minute]
				} else {
					_, seen = previousMinutes[minute]
				}
			}
			if !seen && !session.isMonitorSelf(monitorUser, selfIP) {
				fresh = append(fresh, session)
			}
		}
	}
	return fresh, snapshot
}

// loginMonitor owns its connection and retries transient failures independently
// of the status stream. Its baseline and server clock survive those retries.
type loginMonitor struct {
	serverID int64
	user     string
	dial     func(context.Context) (*gossh.Client, error)
	report   func(context.Context, []loginSession)
	interval time.Duration
}

func (m loginMonitor) run(ctx context.Context) {
	prober := &loginProber{}
	disconnect := func() {
		if prober.client != nil {
			_ = prober.client.Close()
			prober.close()
			prober.client = nil
		}
	}
	defer disconnect()
	interval := m.interval
	if interval <= 0 {
		interval = loginProbeInterval
	}
	backoff := interval
	var snapshot map[string]struct{}
	seeded := false
	failures := 0
	for ctx.Err() == nil {
		var probe loginProbe
		var err error
		if prober.client == nil {
			prober.client, err = m.dial(ctx)
		}
		if err == nil {
			probe, err = prober.probe(ctx)
		}
		delay := interval
		if err != nil {
			disconnect()
			if ctx.Err() != nil {
				return
			}
			if IsPermanentHostKeyError(err) {
				log.Printf("Blocked SSH login connection for server %d: %v", m.serverID, err)
				return
			}
			failures++
			delay = backoff
			backoff = min(backoff*2, loginRetryMaxDelay)
			log.Printf("SSH login probe for server %d failed (attempt %d); retrying in %s: %v", m.serverID, failures, delay, err)
		} else {
			failures = 0
			backoff = interval
			if probe.unavailable {
				log.Printf("SSH login detection unavailable for server %d (`last` missing or wtmp unreadable); login alerts disabled for this connection", m.serverID)
				return
			}

			fresh, current := diffLoginSessions(snapshot, probe.sessions, m.user, probe.selfIP)
			snapshot = current
			if seeded && len(fresh) > 0 {
				m.report(ctx, fresh)
			}
			seeded = true
		}

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// notifyFreshLogins delivers login notifications for newly seen sessions via
// the shared loginnotify reporter.
func notifyFreshLogins(ctx context.Context, serverId, teamID int64, fresh []loginSession) {
	events := make([]loginnotify.Event, 0, len(fresh))
	for _, session := range fresh {
		events = append(events, loginnotify.Event{
			User: session.user,
			TTY:  session.tty,
			IP:   session.ip,
		})
	}
	loginnotify.Report(ctx, serverId, teamID, events)
}
