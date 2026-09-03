package script

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const statusLoopMarker = `cpu_f="$tmpdir/cpu"`

type diskMetric struct {
	MountPoint string  `json:"mp"`
	TotalGB    float64 `json:"total_gb"`
	UsedGB     float64 `json:"used_gb"`
}

func TestDiskSpaceTaskCollectsNetworkMountsAndSkipsMalformedRows(t *testing.T) {
	binDir := t.TempDir()
	writeTestCommand(t, binDir, "timeout", "shift\nexec \"$@\"")
	writeTestCommand(t, binDir, "df", `
printf '%s\n' 'Filesystem 1-blocks Used Available Capacity Mounted on'
printf '%s\n' '/dev/root 4294967296 1073741824 3221225472 25% /'
printf '%s\n' 'server:/share 10737418240 5368709120 5368709120 50% /mnt/team "share"'
printf '%s\n' 'server:/small 1073741824 536870912 536870912 50% /mnt/small'
printf '%s\n' 'server:/bad not-a-number 1 1 1% /mnt/bad'
printf '%s\n' '/dev/boot 10737418240 1 10737418239 1% /boot'
`)

	out := filepath.Join(t.TempDir(), "disks")
	runDiskTask(t, binDir, out, false)

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	value, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "disks=")
	if !ok {
		t.Fatalf("disk output = %q", data)
	}
	var disks []diskMetric
	if err = json.Unmarshal([]byte(value), &disks); err != nil {
		t.Fatalf("invalid disk JSON %q: %v", value, err)
	}
	if len(disks) != 2 {
		t.Fatalf("disks = %#v, want root and network mount", disks)
	}
	if disks[0].MountPoint != "/" || disks[1].MountPoint != `/mnt/team "share"` {
		t.Fatalf("mount points = %#v", disks)
	}
	if disks[1].TotalGB != 10 || disks[1].UsedGB != 5 {
		t.Fatalf("network disk = %#v", disks[1])
	}
}

func TestDiskSpaceTaskPreservesCompleteCacheWhenDFFails(t *testing.T) {
	binDir := t.TempDir()
	writeTestCommand(t, binDir, "timeout", "shift\nexec \"$@\"")
	writeTestCommand(t, binDir, "df", "exit 1")
	out := filepath.Join(t.TempDir(), "disks")
	want := "disks=[{\"mp\":\"/cached\",\"total_gb\":10,\"used_gb\":1}]\n"
	if err := os.WriteFile(out, []byte(want), 0o600); err != nil {
		t.Fatal(err)
	}

	runDiskTask(t, binDir, out, true)

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("cached disk output changed to %q", data)
	}
}

func TestDiskSpaceTaskUsesValidRowsWhenDFPartiallyFails(t *testing.T) {
	binDir := t.TempDir()
	writeTestCommand(t, binDir, "timeout", "shift\nexec \"$@\"")
	writeTestCommand(t, binDir, "df", `
printf '%s\n' 'df: /tmp/com.freerdp.client.cliprdr.1844137: Transport endpoint is not connected' >&2
printf '%s\n' 'Filesystem 1-blocks Used Available Capacity Mounted on'
printf '%s\n' '/dev/sda2 252786221056 150812557312 91586772992 63% /'
printf '%s\n' '/dev/sda1 794296320 9199616 785096704 2% /boot/efi'
exit 1
`)

	out := filepath.Join(t.TempDir(), "disks")
	runDiskTask(t, binDir, out, false)

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	value, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "disks=")
	if !ok {
		t.Fatalf("disk output = %q", data)
	}
	var disks []diskMetric
	if err = json.Unmarshal([]byte(value), &disks); err != nil {
		t.Fatalf("invalid disk JSON %q: %v", value, err)
	}
	if len(disks) != 1 || disks[0].MountPoint != "/" {
		t.Fatalf("disks = %#v, want the valid root filesystem", disks)
	}
}

func TestDiskSpaceTaskPreservesCompleteCacheWhenAwkFails(t *testing.T) {
	binDir := t.TempDir()
	writeTestCommand(t, binDir, "timeout", "shift\nexec \"$@\"")
	writeTestCommand(t, binDir, "df", "exit 0")
	writeTestCommand(t, binDir, "awk", "exit 9")
	out := filepath.Join(t.TempDir(), "disks")
	want := "disks=[{\"mp\":\"/cached\",\"total_gb\":10,\"used_gb\":1}]\n"
	if err := os.WriteFile(out, []byte(want), 0o600); err != nil {
		t.Fatal(err)
	}

	runDiskTask(t, binDir, out, true)

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("cached disk output changed to %q", data)
	}
	if leftovers, err := filepath.Glob(out + ".next.*"); err != nil {
		t.Fatal(err)
	} else if len(leftovers) != 0 {
		t.Fatalf("partial disk files remain: %v", leftovers)
	}
}

func TestRefreshDiskSpaceTaskDoesNotDuplicateRunningCollector(t *testing.T) {
	binDir := t.TempDir()
	writeTestCommand(t, binDir, "timeout", "shift\nexec \"$@\"")
	writeTestCommand(t, binDir, "df", `
sleep 1
printf '%s\n' 'Filesystem 1-blocks Used Available Capacity Mounted on'
printf '%s\n' 'server:/share 10737418240 5368709120 5368709120 50% /mnt/network'
`)
	out := filepath.Join(t.TempDir(), "disks")
	if err := os.WriteFile(out, []byte("disks=[]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	harness := statusScriptFunctions(t) + `
disk_space_f="$1"
refresh_disk_space_task "$disk_space_f"
first_pid="$pid_dspace"
refresh_disk_space_task "$disk_space_f"
if [ "$first_pid" != "$pid_dspace" ]; then
  exit 42
fi
if [ "$(cat "$disk_space_f")" != "disks=[]" ]; then
  exit 43
fi
wait "$pid_dspace"
pid_dspace=""
if ! grep -q '"mp":"/mnt/network"' "$disk_space_f"; then
  exit 44
fi
`
	cmd := exec.Command("bash", "-c", harness, "linux-status-test", out)
	cmd.Env = testCommandEnv(binDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("disk task supervisor failed: %v\n%s", err, output)
	}
}

func TestRefreshDiskSpaceTaskRestartsCompletedCollector(t *testing.T) {
	binDir := t.TempDir()
	writeTestCommand(t, binDir, "timeout", "shift\nexec \"$@\"")
	writeTestCommand(t, binDir, "df", `
printf 'called\n' >> "$DF_CALLS"
printf '%s\n' 'Filesystem 1-blocks Used Available Capacity Mounted on'
`)
	tempDir := t.TempDir()
	out := filepath.Join(tempDir, "disks")
	calls := filepath.Join(tempDir, "calls")
	if err := os.WriteFile(out, []byte("disks=[]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	harness := statusScriptFunctions(t) + `
disk_space_f="$1"
refresh_disk_space_task "$disk_space_f"
attempts=0
while [ ! -f "${disk_space_f}.done" ]; do
  sleep 0.05
  attempts=$((attempts + 1))
  if [ "$attempts" -ge 40 ]; then
    exit 45
  fi
done
refresh_disk_space_task "$disk_space_f"
wait "$pid_dspace"
pid_dspace=""
`
	cmd := exec.Command("bash", "-c", harness, "linux-status-test", out)
	cmd.Env = append(testCommandEnv(binDir), "DF_CALLS="+calls)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("disk task supervisor failed: %v\n%s", err, output)
	}
	data, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(data), "called\n"); got != 2 {
		t.Fatalf("df calls = %d, want a new collection after completion", got)
	}
}

func runDiskTask(t *testing.T, binDir, out string, wantError bool) {
	t.Helper()
	harness := statusScriptFunctions(t) + `
disk_space_task "$1"
`
	cmd := exec.Command("bash", "-c", harness, "linux-status-test", out)
	cmd.Env = testCommandEnv(binDir)
	output, err := cmd.CombinedOutput()
	if wantError && err == nil {
		t.Fatalf("disk task succeeded unexpectedly: %s", output)
	}
	if !wantError && err != nil {
		t.Fatalf("disk task failed: %v\n%s", err, output)
	}
}

func statusScriptFunctions(t *testing.T) string {
	t.Helper()
	contents, err := GetScript("linux_status.sh")
	if err != nil {
		t.Fatal(err)
	}
	prefix, _, ok := strings.Cut(contents, statusLoopMarker)
	if !ok {
		t.Fatalf("status loop marker %q not found", statusLoopMarker)
	}
	return prefix
}

func writeTestCommand(t *testing.T, dir, name, body string) {
	t.Helper()
	contents := "#!/bin/sh\nset -eu\n" + strings.TrimSpace(body) + "\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o755); err != nil {
		t.Fatal(err)
	}
}

func testCommandEnv(binDir string) []string {
	return append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"DISK_SPACE_TIMEOUT=1",
	)
}
