package githubrelease

import "testing"

func TestAssetNameForPlatform(t *testing.T) {
	name, err := AssetNameForPlatform("windows", "amd64")
	if err != nil || name != "agent_windows_amd64.exe" {
		t.Fatalf("got %q %v", name, err)
	}
	_, err = AssetNameForPlatform("freebsd", "amd64")
	if err == nil {
		t.Fatal("expected error")
	}
}
