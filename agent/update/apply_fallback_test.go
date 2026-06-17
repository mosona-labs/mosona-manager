package update

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestApplyIfNeededHubDownloadFallsBackToGitHub(t *testing.T) {
	const remote = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	var calls []string

	prev := stageAndApplyForApply
	stageAndApplyForApply = func(ctx context.Context, downloadURL, wantSHA string) error {
		calls = append(calls, downloadURL)
		if len(calls) == 1 {
			return &DownloadError{Err: errors.New("hub proxy 502")}
		}
		if wantSHA != remote {
			t.Fatalf("fallback sha: %s", wantSHA)
		}
		return nil
	}
	defer func() { stageAndApplyForApply = prev }()

	prevGH := checkViaGitHubForApply
	checkViaGitHubForApply = func(ctx context.Context) (CheckResult, error) {
		return CheckResult{
			UpdateAvailable: true,
			RemoteSHA256:    remote,
			DownloadURL:     "https://github.com/asset/bin",
			Source:          "github",
		}, nil
	}
	defer func() { checkViaGitHubForApply = prevGH }()

	err := ApplyIfNeeded(context.Background(), CheckResult{
		UpdateAvailable: true,
		RemoteSHA256:    remote,
		DownloadURL:     "https://hub.example.com/api/agent/update/download?os=linux&arch=amd64",
		Source:          "hub",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[1] != "https://github.com/asset/bin" {
		t.Fatalf("calls: %v", calls)
	}
}

func TestApplyIfNeededHubNonDownloadErrorNoFallback(t *testing.T) {
	var ghCalled bool
	prev := stageAndApplyForApply
	stageAndApplyForApply = func(ctx context.Context, downloadURL, wantSHA string) error {
		return fmt.Errorf("restart service: failed")
	}
	defer func() { stageAndApplyForApply = prev }()

	prevGH := checkViaGitHubForApply
	checkViaGitHubForApply = func(ctx context.Context) (CheckResult, error) {
		ghCalled = true
		return CheckResult{}, nil
	}
	defer func() { checkViaGitHubForApply = prevGH }()

	err := ApplyIfNeeded(context.Background(), CheckResult{
		UpdateAvailable: true,
		RemoteSHA256:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		DownloadURL:     "https://hub.example.com/api/agent/update/download?os=linux&arch=amd64",
		Source:          "hub",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if ghCalled {
		t.Fatal("github fallback should not run after non-download failure")
	}
}
