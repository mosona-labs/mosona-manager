package ssh

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
	"mosona-manager/internal/connect/ssh/script"
)

func loginTestClient(t *testing.T, handle func(gossh.NewChannel, <-chan struct{})) *gossh.Client {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	config := &gossh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(newTestSigner(t))
	go func() {
		raw, err := listener.Accept()
		if err != nil {
			return
		}
		defer raw.Close()
		conn, channels, requests, err := gossh.NewServerConn(raw, config)
		if err != nil {
			return
		}
		defer conn.Close()
		go gossh.DiscardRequests(requests)
		closed := make(chan struct{})
		go func() { _ = conn.Wait(); close(closed) }()
		for channel := range channels {
			go handle(channel, closed)
		}
	}()
	client, err := gossh.Dial("tcp", listener.Addr().String(), &gossh.ClientConfig{
		User: "test", HostKeyCallback: gossh.InsecureIgnoreHostKey(), Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestLoginProbeReusesShellAndRemoteClock(t *testing.T) {
	for _, skew := range []time.Duration{-10 * time.Minute, 10 * time.Minute} {
		t.Run(skew.String(), func(t *testing.T) {
			epoch := time.Now().Add(skew).Unix()
			var starts atomic.Int32
			requests := make(chan string, 4)
			finished := make(chan struct{})
			client := loginTestClient(t, func(next gossh.NewChannel, _ <-chan struct{}) {
				ch, reqs, err := next.Accept()
				if err != nil {
					return
				}
				defer ch.Close()
				for req := range reqs {
					if req.Type != "exec" {
						_ = req.Reply(false, nil)
						continue
					}
					starts.Add(1)
					_ = req.Reply(true, nil)
					scanner := bufio.NewScanner(ch)
					for scanner.Scan() {
						requests <- scanner.Text()
						_, _ = fmt.Fprintf(ch, "server_epoch=%d\nwtmp_records=1100\nlogin|root|pts/0|192.0.2.1|stable date\n%s0\n", epoch, loginProbeEnd)
					}
					close(finished)
					return
				}
			})
			prober := &loginProber{client: client}
			defer prober.close()
			var snapshot map[string]struct{}
			for i := 0; i < 3; i++ {
				probe, err := prober.probe(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				got := <-requests
				var wantRecords, wantSince int64
				if i > 0 {
					wantRecords = 1100 - loginWtmpOverlap
					wantSince = epoch - int64(loginSinceOverlap/time.Second)
				}
				if got != fmt.Sprintf("%d %d %d", wantRecords, wantSince, loginLastLines) {
					t.Fatalf("poll must transmit only server-side cursors, got %q", got)
				}
				fresh, current := diffLoginSessions(snapshot, probe.sessions, "monitor", "")
				snapshot = current
				if len(fresh) != 0 {
					t.Fatalf("unchanged logins reported again: %v", fresh)
				}
			}
			if starts.Load() != 1 {
				t.Fatalf("uploaded script %d times", starts.Load())
			}
			prober.close()
			select {
			case <-finished:
			case <-time.After(time.Second):
				t.Fatal("closing prober did not terminate remote input loop")
			}
		})
	}
}

func TestLoginProbeShellExecutesMultipleRequests(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"date": "echo 1700000300",
		"last": `printf '%s\n' "$*" >> "$LOGIN_TEST_ARGS"
printf '%s\n' 'root pts/0 192.0.2.1 Mon Jan 5 09:30:00 2026 still logged in'`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	contents, err := script.GetScript("linux_logins.sh")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", loginProbeCommand(contents))
	cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"),
		"LOGIN_TEST_ARGS="+filepath.Join(dir, "args"),
		"LOGIN_WTMP="+filepath.Join(dir, "no-wtmp"))
	cmd.Stdin = strings.NewReader("0 0 50\n0 1700000000 50\n")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("persistent shell: %v: %s", err, output)
	}
	if strings.Count(string(output), loginProbeEnd+"0\n") != 2 || strings.Count(string(output), "server_epoch=1700000300") != 2 {
		t.Fatalf("probe exit/traps broke subsequent requests: %s", output)
	}
	args, err := os.ReadFile(filepath.Join(dir, "args"))
	if err != nil {
		t.Fatal(err)
	}
	if string(args) != "-F -n 50 -i\n-F -n 50 -i -s @1700000000\n" {
		t.Fatalf("unexpected probe arguments: %s", args)
	}
}

func TestLoginProbeDeduplicatesWtmpAndLastScriptOutput(t *testing.T) {
	dir := t.TempDir()
	wtmp := filepath.Join(dir, "wtmp")
	if err := os.WriteFile(wtmp, make([]byte, 384), 0600); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"stat": `wc -c < "$3" | tr -d ' '`,
		"utmpdump": `
[ "$TZ" = UTC0 ] || exit 99
if [ "$1" = -r ]; then
  [ "$LOGIN_TEST_SOURCE" = wtmp ] || exit 1
  dd if=/dev/zero bs=384 count=1 2>/dev/null
  exit 0
fi
echo '[7] [01072] [ts/0] [root    ] [pts/0       ] [gateway             ] [192.0.2.1      ] [2026-10-08T12:00:23,123456+00:00]'`,
		"last": `
[ "$TZ" = UTC0 ] || exit 99
echo 'root pts/0 192.0.2.1 Thu Oct 8 12:00:23 2026 still logged in'`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nset -eu\n"+body+"\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	contents, err := script.GetScript("linux_logins.sh")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot map[string]struct{}
	for _, source := range []string{"wtmp", "last", "wtmp"} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		cmd := exec.CommandContext(ctx, "sh", "-c", contents)
		cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"),
			"TZ=Asia/Hong_Kong", "LOGIN_WTMP="+wtmp, "LOGIN_TEST_SOURCE="+source)
		output, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("script using %s: %v: %s", source, err, output)
		}
		probe := parseLoginProbe(string(output))
		if len(probe.sessions) != 1 {
			t.Fatalf("%s did not return the login: %s", source, output)
		}
		fresh, next := diffLoginSessions(snapshot, probe.sessions, "monitor", "")
		if len(fresh) != 0 {
			t.Fatalf("switch to %s re-reported the login: %#v", source, fresh)
		}
		snapshot = next
	}
}

func TestLoginProbeDeadlineUnblocksEveryStage(t *testing.T) {
	for _, stage := range []string{"open", "exec", "response"} {
		t.Run(stage, func(t *testing.T) {
			reached := make(chan struct{})
			client := loginTestClient(t, func(next gossh.NewChannel, closed <-chan struct{}) {
				if stage == "open" {
					close(reached)
					<-closed
					return
				}
				ch, reqs, err := next.Accept()
				if err != nil {
					return
				}
				defer ch.Close()
				for req := range reqs {
					if stage == "exec" {
						close(reached)
						<-closed
						return
					}
					_ = req.Reply(true, nil)
					_, _ = bufio.NewReader(ch).ReadString('\n')
					_, _ = fmt.Fprintf(ch, "%slast\n", loginProbeStage)
					close(reached)
					<-closed
					return
				}
			})
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			prober := &loginProber{client: client}
			defer prober.close()
			_, err := prober.probe(ctx)
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("error = %v, want deadline exceeded", err)
			}
			wantStage := map[string]string{
				"open": "opening SSH session", "exec": "starting remote shell", "response": "reading login history (last)",
			}[stage]
			if !strings.Contains(err.Error(), wantStage) {
				t.Fatalf("error = %v, want blocked stage %q", err, wantStage)
			}
			select {
			case <-reached:
			default:
				t.Fatalf("probe timed out before reaching %s", stage)
			}
			if _, err := client.NewSession(); err == nil {
				t.Fatal("stalled connection was not closed for reconnect")
			}
		})
	}
}

func TestLoginProbeShellContinuesAfterRemoteTimeout(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"date": "echo 1700000300",
		"last": "echo 'root pts/0 192.0.2.1 Mon Jan 5 09:30:00 2026 still logged in'",
		"timeout": `
[ "$1 $2 $3 $4" = '-k 1 10 last' ] || exit 99
if [ ! -f "$LOGIN_TEST_TIMEOUT" ]; then
  touch "$LOGIN_TEST_TIMEOUT"
  exit 124
fi
shift 3
exec "$@"`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	contents, err := script.GetScript("linux_logins.sh")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", loginProbeCommand(contents))
	cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"),
		"LOGIN_TEST_TIMEOUT="+filepath.Join(dir, "timed-out"),
		"LOGIN_WTMP="+filepath.Join(dir, "no-wtmp"))
	cmd.Stdin = strings.NewReader("0 0 50\n0 1700000000 50\n")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("persistent shell: %v: %s", err, output)
	}
	if strings.Count(string(output), loginProbeEnd+"124\n") != 1 || strings.Count(string(output), loginProbeEnd+"0\n") != 1 {
		t.Fatalf("timeout was swallowed or stopped subsequent requests: %s", output)
	}
}

// A slow last must not invalidate the healthy status connection. The first
// successful probe is a baseline; after a timeout the next connection must
// retain both the deduplication snapshot and the target's clock.
func TestLoginMonitorTimeoutReconnectsIndependentlyAndPreservesBaseline(t *testing.T) {
	statusClient := loginTestClient(t, func(next gossh.NewChannel, _ <-chan struct{}) {
		ch, requests, err := next.Accept()
		if err != nil {
			return
		}
		defer ch.Close()
		for req := range requests {
			_ = req.Reply(true, nil)
			_, _ = fmt.Fprint(ch, "healthy status\n")
			_, _ = ch.SendRequest("exit-status", false, gossh.Marshal(struct{ Status uint32 }{0}))
			return
		}
	})
	const epoch = int64(1700000300)
	args := make(chan string, 8)
	newLoginClient := func(timeout bool) *gossh.Client {
		return loginTestClient(t, func(next gossh.NewChannel, _ <-chan struct{}) {
			ch, requests, err := next.Accept()
			if err != nil {
				return
			}
			defer ch.Close()
			for req := range requests {
				_ = req.Reply(true, nil)
				scanner := bufio.NewScanner(ch)
				poll := 0
				for scanner.Scan() {
					poll++
					args <- scanner.Text()
					if timeout && poll == 2 {
						_, _ = fmt.Fprintf(ch, "%slast\n%s124\n", loginProbeStage, loginProbeEnd)
						continue
					}
					_, _ = fmt.Fprintf(ch, "server_epoch=%d\nlogin|root|pts/0|192.0.2.1|old\n", epoch)
					if !timeout {
						_, _ = fmt.Fprint(ch, "login|deploy|pts/1|192.0.2.2|new\n")
					}
					_, _ = fmt.Fprintf(ch, "%s0\n", loginProbeEnd)
				}
				return
			}
		})
	}
	first, second := newLoginClient(true), newLoginClient(false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var dials atomic.Int32
	events := make(chan []loginSession, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		(loginMonitor{
			user: "monitor",
			dial: func(context.Context) (*gossh.Client, error) {
				switch dials.Add(1) {
				case 1:
					return first, nil
				case 2:
					return second, nil
				default:
					return nil, errors.New("unexpected extra dial")
				}
			},
			interval: time.Millisecond,
			report: func(_ context.Context, fresh []loginSession) {
				events <- fresh
				cancel()
			},
		}).run(ctx)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("login stream did not recover and stop")
	}
	if dials.Load() != 2 {
		t.Fatalf("dials = %d, want 2", dials.Load())
	}
	select {
	case fresh := <-events:
		if len(fresh) != 1 || fresh[0].user != "deploy" {
			t.Fatalf("reported %v, want only new login", fresh)
		}
	default:
		t.Fatal("new login after reconnect was lost")
	}
	for _, want := range []string{"0 0 50", "0 1700000000 50", "0 1700000000 50"} {
		select {
		case got := <-args:
			if got != want {
				t.Fatalf("probe args = %q, want %q", got, want)
			}
		default:
			t.Fatalf("missing probe request %q", want)
		}
	}
	for _, client := range []*gossh.Client{first, second} {
		if _, err := client.NewSession(); err == nil {
			t.Fatal("login connection leaked after timeout/cancellation")
		}
	}
	session, err := statusClient.NewSession()
	if err != nil {
		t.Fatalf("status connection was closed by login monitor: %v", err)
	}
	defer session.Close()
	output, err := session.Output("status")
	if err != nil || string(output) != "healthy status\n" {
		t.Fatalf("status after login timeout = %q, %v", output, err)
	}
}

func TestLoginMonitorRetriesBeyondFiveFailuresAndCancelsDial(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reached := make(chan struct{})
	done := make(chan struct{})
	var attempts atomic.Int32
	go func() {
		defer close(done)
		(loginMonitor{
			interval: time.Millisecond,
			dial: func(ctx context.Context) (*gossh.Client, error) {
				if attempts.Add(1) < 7 {
					return nil, errors.New("temporary connection error")
				}
				close(reached)
				<-ctx.Done()
				return nil, ctx.Err()
			},
		}).run(ctx)
	}()
	select {
	case <-reached:
	case <-time.After(2 * time.Second):
		t.Fatal("login monitor stopped retrying transient failures")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("login dial did not stop on cancellation")
	}
}

func TestLoginMonitorStopsOnHostKeyMismatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	attempts := 0
	(loginMonitor{
		interval: time.Millisecond,
		dial: func(context.Context) (*gossh.Client, error) {
			attempts++
			return nil, ErrHostKeyMismatch
		},
	}).run(ctx)
	if attempts != 1 || ctx.Err() != nil {
		t.Fatalf("host key mismatch must stop immediately; attempts=%d, context=%v", attempts, ctx.Err())
	}
}
