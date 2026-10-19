package script

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func runLoginsScript(t *testing.T, binDir string, args []string, env ...string) string {
	t.Helper()
	writeTestCommand(t, binDir, "date", "echo 1700000300")
	contents, err := GetScript("linux_logins.sh")
	if err != nil {
		t.Fatal(err)
	}
	argv := append([]string{"sh", "-c", contents, "linux-logins-test"}, args...)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmdEnv := append(testCommandEnv(binDir), env...)
	hasWtmp := false
	for _, entry := range cmdEnv {
		if strings.HasPrefix(entry, "LOGIN_WTMP=") {
			hasWtmp = true
		}
	}
	// Tests that do not exercise the incremental wtmp path must not depend on
	// the host having (or lacking) a readable /var/log/wtmp.
	if !hasWtmp {
		cmdEnv = append(cmdEnv, "LOGIN_WTMP="+filepath.Join(binDir, "wtmp-missing"))
	}
	cmd.Env = cmdEnv
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("linux_logins.sh failed: %v\n%s", err, output)
	}
	return string(output)
}

// writeFakeWtmp creates a file of n zero-filled 384-byte utmp records. The
// fast path only reads record counts from it; utmpdump is stubbed in tests.
func writeFakeWtmp(t *testing.T, records int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "wtmp")
	if err := os.WriteFile(path, make([]byte, records*384), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readCapturedArgs(t *testing.T, binDir string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(binDir, "last.args"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(data))
}

func TestLinuxLoginsParsesLastOutput(t *testing.T) {
	binDir := t.TempDir()
	writeTestCommand(t, binDir, "last", `
if [ "${1:-}" = "-F" ]; then
  printf '%s\n' 'root     pts/0        203.0.113.7     Fri Oct  8 12:00:23 2026   still logged in'
  printf '%s\n' 'deploy   pts/1        2001:db8::1     Fri Oct  8 11:00:00 2026 - Fri Oct  8 11:30:00 2026  (00:30)'
  printf '%s\n' 'admin    tty1                          Fri Oct  8 09:00:00 2026   still logged in'
  printf '%s\n' 'reboot   system boot  6.1.0-amd64     Fri Oct  8 08:00:00 2026   still running'
  printf '%s\n' ''
  printf '%s\n' 'wtmp begins Fri Oct  1 00:00:00 2026'
  exit 0
fi
exit 1
`)

	output := runLoginsScript(t, binDir, nil, "SSH_CONNECTION=198.51.100.7 51234 10.0.0.1 22")

	want := strings.Join([]string{
		"self_ip=198.51.100.7",
		"login|root|pts/0|203.0.113.7|Fri Oct 8 12:00:23 2026",
		"login|deploy|pts/1|2001:db8::1|Fri Oct 8 11:00:00 2026",
		"server_epoch=1700000300",
		"",
	}, "\n")
	if output != want {
		t.Fatalf("linux_logins.sh output = %q, want %q", output, want)
	}
}

func TestLinuxLoginsPassesSinceEpochToLast(t *testing.T) {
	binDir := t.TempDir()
	writeTestCommand(t, binDir, "last", `
cd "$(dirname "$0")"
for arg in "$@"; do
  printf '%s ' "$arg" >> last.args
done
printf '\n' >> last.args
printf '%s\n' 'root     pts/0        203.0.113.7     Fri Oct  8 12:00:23 2026   still logged in'
exit 0
`)

	output := runLoginsScript(t, binDir, []string{"0", "1700000000", "50"}, "SSH_CONNECTION=198.51.100.7 51234 10.0.0.1 22")

	args := readCapturedArgs(t, binDir)
	hasSince := false
	for i, arg := range args {
		if arg == "-s" && i+1 < len(args) && args[i+1] == "@1700000000" {
			hasSince = true
		}
	}
	if !hasSince {
		t.Fatalf("last args = %v, want -s @1700000000", args)
	}
	if !strings.Contains(output, "login|root|pts/0|203.0.113.7|Fri Oct 8 12:00:23 2026") {
		t.Fatalf("linux_logins.sh output missing login record: %q", output)
	}
}

func TestLinuxLoginsFallsBackWhenFullTimesUnsupported(t *testing.T) {
	binDir := t.TempDir()
	writeTestCommand(t, binDir, "last", `
for arg in "$@"; do
  if [ "$arg" = "-s" ] || [ "$arg" = "-F" ]; then
    exit 1
  fi
done
printf '%s\n' 'root     pts/2        192.0.2.9      Mon Jan  5 09:30   still logged in'
exit 0
`)

	output := runLoginsScript(t, binDir, []string{"0", "1700000000", "50"}, "SSH_CONNECTION=198.51.100.7 51234 10.0.0.1 22")

	want := "self_ip=198.51.100.7\nlogin|root|pts/2|192.0.2.9|Mon Jan 5 09:30\nserver_epoch=1700000300\n"
	if output != want {
		t.Fatalf("linux_logins.sh output = %q, want %q (short-format date must not include session status)", output, want)
	}
}

func TestLinuxLoginsIdleSinceWindowIsEmpty(t *testing.T) {
	binDir := t.TempDir()
	writeTestCommand(t, binDir, "last", `
for arg in "$@"; do
  if [ "$arg" = "-s" ]; then
    exit 0
  fi
done
exit 1
`)

	output := runLoginsScript(t, binDir, []string{"0", "1700000000", "50"}, "SSH_CONNECTION=198.51.100.7 51234 10.0.0.1 22")

	if output != "self_ip=198.51.100.7\nserver_epoch=1700000300\n" {
		t.Fatalf("linux_logins.sh output = %q, want only probe metadata", output)
	}
}

func TestLinuxLoginsReportsUnavailableWhenLastFails(t *testing.T) {
	binDir := t.TempDir()
	writeTestCommand(t, binDir, "last", "exit 1")

	output := runLoginsScript(t, binDir, nil, "SSH_CONNECTION=198.51.100.7 51234 10.0.0.1 22")

	want := "self_ip=198.51.100.7\nlogins_unavailable=1\n"
	if output != want {
		t.Fatalf("linux_logins.sh output = %q, want %q", output, want)
	}
}

func TestLinuxLoginsEmptySSHConnection(t *testing.T) {
	binDir := t.TempDir()
	writeTestCommand(t, binDir, "last", "exit 1")

	cmdEnv := testCommandEnv(binDir)
	for i, entry := range cmdEnv {
		if strings.HasPrefix(entry, "SSH_CONNECTION=") {
			cmdEnv[i] = "SSH_CONNECTION="
		}
	}
	contents, err := GetScript("linux_logins.sh")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", contents, "linux-logins-test")
	cmd.Env = append(cmdEnv, "LOGIN_WTMP="+filepath.Join(binDir, "wtmp-missing"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("linux_logins.sh failed: %v\n%s", err, output)
	}
	if !strings.HasPrefix(string(output), "self_ip=\n") {
		t.Fatalf("linux_logins.sh output should start with empty self_ip: %q", output)
	}
	if !strings.Contains(string(output), "logins_unavailable=1") {
		t.Fatalf("linux_logins.sh output missing logins_unavailable: %q", output)
	}
}

// dash is /bin/sh on Debian/Ubuntu and does not support bashisms such as
// (( ... )); run the idle-window scenario through it so a silently
// misbehaving fallback gate cannot leak full login history into an
// incremental poll again.
func TestLinuxLoginsIdleSinceWindowIsEmptyUnderDash(t *testing.T) {
	dashPath, err := exec.LookPath("dash")
	if err != nil {
		t.Skip("dash not available")
	}

	binDir := t.TempDir()
	writeTestCommand(t, binDir, "last", `
for arg in "$@"; do
  if [ "$arg" = "-s" ]; then
    exit 0
  fi
done
printf '%s\n' 'root     pts/2        192.0.2.9      Mon Jan  5 09:30   still logged in'
exit 0
`)

	contents, err := GetScript("linux_logins.sh")
	if err != nil {
		t.Fatal(err)
	}
	writeTestCommand(t, binDir, "date", "echo 1700000300")
	cmd := exec.Command(dashPath, "-c", contents, "linux-logins-test", "0", "1700000000", "50")
	cmd.Env = append(testCommandEnv(binDir),
		"SSH_CONNECTION=198.51.100.7 51234 10.0.0.1 22",
		"LOGIN_WTMP="+filepath.Join(binDir, "wtmp-missing"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("linux_logins.sh under dash failed: %v\n%s", err, output)
	}
	if string(output) != "self_ip=198.51.100.7\nserver_epoch=1700000300\n" {
		t.Fatalf("linux_logins.sh under dash output = %q, want only probe metadata", output)
	}
}

func TestLinuxLoginsSinceFallbackPreservesSessionKeys(t *testing.T) {
	binDir := t.TempDir()
	writeTestCommand(t, binDir, "last", `
full=0
for arg in "$@"; do
  case "$arg" in
    -s) exit 1 ;;
    -F) full=1 ;;
  esac
done
if [ "$full" -eq 1 ]; then
  printf '%s\n' 'root pts/2 192.0.2.9 Mon Jan 5 09:30:00 2026 still logged in'
else
  printf '%s\n' 'root pts/2 192.0.2.9 Mon Jan 5 09:30 still logged in'
fi
`)
	baseline := runLoginsScript(t, binDir, nil)
	incremental := runLoginsScript(t, binDir, []string{"0", "1700000000", "50"})
	if incremental != baseline || !strings.Contains(incremental, "09:30:00 2026") {
		t.Fatalf("same login changed keys: baseline %q, incremental %q", baseline, incremental)
	}
}

func TestLinuxLoginsDoesNotRetryTimedOutLast(t *testing.T) {
	for _, status := range []string{"124", "137", "143"} {
		t.Run(status, func(t *testing.T) {
			binDir := t.TempDir()
			writeTestCommand(t, binDir, "date", "echo 1700000300")
			writeTestCommand(t, binDir, "last", "exit 99")
			writeTestCommand(t, binDir, "timeout", `
printf '%s\n' "$*" >> "$LOGIN_TEST_ARGS"
# Partial output must not be reported as a successful snapshot.
printf '%s\n' 'root pts/0 192.0.2.1 Mon Jan 5 09:30:00 2026 still logged in'
exit `+status)
			contents, err := GetScript("linux_logins.sh")
			if err != nil {
				t.Fatal(err)
			}
			argsPath := filepath.Join(binDir, "args")
			cmd := exec.Command("sh", "-c", contents, "test", "0", "1700000000", "50")
			cmd.Env = append(testCommandEnv(binDir),
				"LOGIN_TEST_ARGS="+argsPath,
				"LOGIN_WTMP="+filepath.Join(binDir, "wtmp-missing"))
			output, err := cmd.CombinedOutput()
			exitErr, ok := err.(*exec.ExitError)
			if !ok || strconv.Itoa(exitErr.ExitCode()) != status {
				t.Fatalf("probe error = %v, want exit %s; output=%s", err, status, output)
			}
			if strings.Contains(string(output), "server_epoch=") || strings.Contains(string(output), "login|") || strings.Contains(string(output), "logins_unavailable=") {
				t.Fatalf("timed out snapshot was marked complete or unavailable: %s", output)
			}
			args, err := os.ReadFile(argsPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(args) != "-k 1 10 last -F -n 50 -i -s @1700000000\n" {
				t.Fatalf("slow command was retried or deadline flags changed: %s", args)
			}
		})
	}
}

// utmpdumpStub prints canned bracketed records and logs the byte size of the
// file it was asked to dump, so tests can assert how much of wtmp was read.
const utmpdumpStub = `
if [ "$1" = -r ]; then
  dd if=/dev/zero bs=384 count=1 2>/dev/null
  exit 0
fi
wc -c < "$1" | tr -d ' ' >> "$LOGIN_TEST_ARGS"
printf '%s\n' 'Utmp dump of test'
printf '%s\n' '[2] [00000] [~~  ] [reboot      ] [~           ] [6.1.0-amd64    ] [0.0.0.0        ] [2026-10-08T00:00:00,000000+00:00]'
printf '%s\n' '[7] [01072] [ts/0] [root        ] [pts/0       ] [192.0.2.1      ] [192.0.2.1      ] [2026-10-08T12:00:23,000000+00:00]'
printf '%s\n' '[7] [01073] [ts/1] [deploy      ] [pts/1       ] [gateway        ] [0.0.0.0        ] [2026-10-08T12:01:00,000000+00:00]'
`

// statStub answers `stat -c %s FILE` portably, since the BSD stat on
// developer machines has no -c flag.
const statStub = `
shift $(($# - 1))
wc -c < "$1" | tr -d ' '
`

func TestLinuxLoginsReadsWtmpIncrementally(t *testing.T) {
	binDir := t.TempDir()
	wtmp := writeFakeWtmp(t, 10)
	// `last` must not be consulted on the incremental path.
	writeTestCommand(t, binDir, "last", "exit 1")
	writeTestCommand(t, binDir, "stat", statStub)
	writeTestCommand(t, binDir, "utmpdump", utmpdumpStub)

	argsPath := filepath.Join(binDir, "args")
	output := runLoginsScript(t, binDir, []string{"6", "1700000000", "50"},
		"SSH_CONNECTION=198.51.100.7 51234 10.0.0.1 22",
		"LOGIN_WTMP="+wtmp,
		"LOGIN_TEST_ARGS="+argsPath)

	want := strings.Join([]string{
		"self_ip=198.51.100.7",
		// Padding inside utmpdump's [brackets] must be trimmed, and entries
		// without an IP literal (gateway) must be dropped.
		"login|root|pts/0|192.0.2.1|2026-10-08T12:00:23,000000+00:00",
		"wtmp_records=10",
		"server_epoch=1700000300",
		"",
	}, "\n")
	if output != want {
		t.Fatalf("linux_logins.sh output = %q, want %q", output, want)
	}
	// The Hub cursor covers 6 records, so dd must skip them: (10-6)*384.
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(args) != "1536\n" {
		t.Fatalf("incremental read size = %q, want 1536 (4 records)", args)
	}
}

func TestLinuxLoginsFirstPollReadsBoundedTailWindow(t *testing.T) {
	binDir := t.TempDir()
	wtmp := writeFakeWtmp(t, 1000)
	writeTestCommand(t, binDir, "last", "exit 1")
	writeTestCommand(t, binDir, "stat", statStub)
	writeTestCommand(t, binDir, "utmpdump", utmpdumpStub)

	argsPath := filepath.Join(binDir, "args")
	output := runLoginsScript(t, binDir, []string{"0", "1700000000", "50"},
		"LOGIN_WTMP="+wtmp,
		"LOGIN_TEST_ARGS="+argsPath)

	if !strings.Contains(output, "wtmp_records=1000\n") {
		t.Fatalf("output missing record count: %q", output)
	}
	// No cursor: only the tail window (default 256 records) is read.
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(args) != "98304\n" {
		t.Fatalf("tail window read size = %q, want 98304 (256 records)", args)
	}
}

func TestLinuxLoginsWtmpRotationRewindsToTailWindow(t *testing.T) {
	binDir := t.TempDir()
	wtmp := writeFakeWtmp(t, 10)
	writeTestCommand(t, binDir, "last", "exit 1")
	writeTestCommand(t, binDir, "stat", statStub)
	writeTestCommand(t, binDir, "utmpdump", utmpdumpStub)

	argsPath := filepath.Join(binDir, "args")
	output := runLoginsScript(t, binDir, []string{"999", "1700000000", "50"},
		"LOGIN_WTMP="+wtmp,
		"LOGIN_TEST_ARGS="+argsPath)

	if !strings.Contains(output, "wtmp_records=10\n") {
		t.Fatalf("output missing record count: %q", output)
	}
	// Cursor beyond the file (rotation): rewind to the bounded tail window,
	// which here covers the whole 10-record file.
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(args) != "3840\n" {
		t.Fatalf("post-rotation read size = %q, want 3840 (10 records)", args)
	}
}

func TestLinuxLoginsMisalignedWtmpFallsBackToLast(t *testing.T) {
	binDir := t.TempDir()
	// 100 bytes is not a whole number of 384-byte records.
	wtmp := writeFakeWtmp(t, 0)
	if err := os.WriteFile(wtmp, make([]byte, 100), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTestCommand(t, binDir, "last", `
printf '%s\n' 'root     pts/0        203.0.113.7     Fri Oct  8 12:00:23 2026   still logged in'
exit 0
`)
	writeTestCommand(t, binDir, "utmpdump", "exit 9")
	writeTestCommand(t, binDir, "stat", statStub)

	output := runLoginsScript(t, binDir, []string{"0", "1700000000", "50"}, "LOGIN_WTMP="+wtmp)

	if !strings.Contains(output, "login|root|pts/0|203.0.113.7|") {
		t.Fatalf("expected last fallback output, got %q", output)
	}
	if strings.Contains(output, "wtmp_records=") {
		t.Fatalf("fallback must not report a wtmp cursor: %q", output)
	}
}

func TestLinuxLoginsUsesNativeWtmpRecordSize(t *testing.T) {
	for _, recordSize := range []int{384, 400} {
		t.Run(strconv.Itoa(recordSize), func(t *testing.T) {
			binDir := t.TempDir()
			wtmp := filepath.Join(binDir, "wtmp")
			// 600 * 400 is also divisible by 384. Check record boundaries,
			// not just divisibility, when the script extracts the tail.
			data := make([]byte, 600*recordSize)
			for i := 0; i < 600; i++ {
				data[i*recordSize] = 7
			}
			if err := os.WriteFile(wtmp, data, 0600); err != nil {
				t.Fatal(err)
			}
			writeTestCommand(t, binDir, "stat", statStub)
			writeTestCommand(t, binDir, "last", "exit 99")
			writeTestCommand(t, binDir, "utmpdump", fmt.Sprintf(`
if [ "$1" = -r ]; then
  dd if=/dev/zero bs=%d count=1 2>/dev/null
  exit 0
fi
[ "$(wc -c < "$1" | tr -d ' ')" = %d ] || exit 1
[ "$(od -An -tu1 -N1 "$1" | tr -d ' ')" = 7 ] || exit 1
printf '%%s\n' '[7] [01072] [ts/0] [root    ] [pts/0       ] [gateway             ] [192.0.2.1      ] [2026-10-08T12:00:23,000000+00:00]'
`, recordSize, 256*recordSize))
			output := runLoginsScript(t, binDir, nil, "LOGIN_WTMP="+wtmp)
			if !strings.Contains(output, "wtmp_records=600\n") || !strings.Contains(output, "login|root|pts/0|192.0.2.1|") {
				t.Fatalf("native %d-byte records were not read correctly: %s", recordSize, output)
			}
		})
	}
}

func TestLinuxLoginsWtmpFailureDoesNotAdvanceCursor(t *testing.T) {
	for _, failure := range []string{"reverse", "unknown-size", "decode", "misaligned"} {
		t.Run(failure, func(t *testing.T) {
			binDir := t.TempDir()
			wtmp := writeFakeWtmp(t, 10)
			if failure == "misaligned" {
				if err := os.Truncate(wtmp, 385); err != nil {
					t.Fatal(err)
				}
			}
			writeTestCommand(t, binDir, "stat", statStub)
			writeTestCommand(t, binDir, "utmpdump", `
if [ "$1" = -r ]; then
  [ "$LOGIN_TEST_FAILURE" != reverse ] || exit 1
  size=384
  [ "$LOGIN_TEST_FAILURE" != unknown-size ] || size=512
  dd if=/dev/zero bs="$size" count=1 2>/dev/null
  exit 0
fi
# Even partial decoder output must not escape into a successful snapshot.
echo '[7] [01072] [ts/0] [bad     ] [pts/1       ] [192.0.2.2           ] [192.0.2.2      ] [2026-10-08T12:00:23,000000+00:00]'
exit 1
`)
			writeTestCommand(t, binDir, "last", "echo 'root pts/0 192.0.2.1 Thu Oct 8 12:00:23 2026 still logged in'")
			output := runLoginsScript(t, binDir, nil, "LOGIN_WTMP="+wtmp, "LOGIN_TEST_FAILURE="+failure)
			if !strings.Contains(output, "login|root|pts/0|192.0.2.1|") || strings.Contains(output, "wtmp_records=") || strings.Contains(output, "login|bad|") {
				t.Fatalf("failed wtmp read was not safely replaced by last: %s", output)
			}
		})
	}
}

func TestLinuxLoginsPrefersNumericWtmpAddress(t *testing.T) {
	for _, tc := range []struct{ host, address, want string }{
		{"gateway", "192.0.2.1", "192.0.2.1"},
		{"gateway.example.org", "192.0.2.1", "192.0.2.1"},
		{"gateway", "2001:db8::1", "2001:db8::1"},
		{"192.0.2.2", "0.0.0.0", "192.0.2.2"},
		{"2001:db8::2", "::", "2001:db8::2"},
		{"gateway.example.org", "0.0.0.0", ""},
		{"gateway", "::", ""},
		{"0.0.0.0", "0.0.0.0", ""},
	} {
		t.Run(tc.host+"/"+tc.address, func(t *testing.T) {
			binDir := t.TempDir()
			wtmp := writeFakeWtmp(t, 1)
			writeTestCommand(t, binDir, "stat", statStub)
			writeTestCommand(t, binDir, "utmpdump", fmt.Sprintf(`
if [ "$1" = -r ]; then
  dd if=/dev/zero bs=384 count=1 2>/dev/null
  exit 0
fi
printf '%%s\n' '[7] [01072] [ts/0] [root    ] [pts/0       ] [%s   ] [%s   ] [2026-10-08T12:00:23,000000+00:00]'
`, tc.host, tc.address))
			output := runLoginsScript(t, binDir, nil, "LOGIN_WTMP="+wtmp)
			if tc.want == "" {
				if strings.Contains(output, "login|") {
					t.Fatalf("non-address was reported: %s", output)
				}
			} else if !strings.Contains(output, "login|root|pts/0|"+tc.want+"|") {
				t.Fatalf("numeric address %s missing: %s", tc.want, output)
			}
		})
	}
}
