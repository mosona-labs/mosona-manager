package update

import (
	"context"
	"fmt"
	"log"
	"strings"
)

func ApplyIfNeeded(ctx context.Context, check CheckResult) error {
	if !check.UpdateAvailable {
		return nil
	}
	if check.DownloadURL == "" || check.RemoteSHA256 == "" {
		return fmt.Errorf("incomplete update metadata")
	}
	if check.Source == "hub" {
		if err := stageAndApplyForApply(ctx, check.DownloadURL, check.RemoteSHA256); err != nil {
			if !IsDownloadError(err) {
				return err
			}
			log.Printf("auto-update: hub download failed, falling back to GitHub: %v", err)
			gh, ghErr := checkViaGitHubForApply(ctx)
			if ghErr != nil {
				return fmt.Errorf("hub apply: %w; github fallback check: %v", err, ghErr)
			}
			if !gh.UpdateAvailable {
				return fmt.Errorf("hub apply: %w; github reports up to date", err)
			}
			if gh.DownloadURL == "" {
				return fmt.Errorf("hub apply: %w; github fallback missing download url", err)
			}
			if !strings.EqualFold(gh.RemoteSHA256, check.RemoteSHA256) {
				log.Printf("auto-update: github fallback sha256 differs from hub metadata, using github")
			}
			return stageAndApplyForApply(ctx, gh.DownloadURL, gh.RemoteSHA256)
		}
		return nil
	}
	return stageAndApplyForApply(ctx, check.DownloadURL, check.RemoteSHA256)
}

var (
	stageAndApplyForApply  = stageAndApply
	checkViaGitHubForApply = checkViaGitHub
)

func CheckAndApply(ctx context.Context) (CheckResult, error) {
	res, err := Check(ctx)
	if err != nil {
		return res, err
	}
	if err := ApplyIfNeeded(ctx, res); err != nil {
		return res, err
	}
	return res, nil
}
