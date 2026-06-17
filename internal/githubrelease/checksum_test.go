package githubrelease

import "testing"

func TestParseChecksumFile(t *testing.T) {
	sum, err := ParseChecksumFile([]byte("abc123  agent_linux_amd64\n"))
	if err == nil || sum != "" {
		t.Fatalf("expected error for short hash, got %q %v", sum, err)
	}
	valid := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	got, err := ParseChecksumFile([]byte(valid + "  file\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got != valid {
		t.Fatalf("got %q", got)
	}
}
