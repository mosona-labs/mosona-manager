package update

import (
	"context"
	"fmt"
)

func ApplyIfNeeded(ctx context.Context, check CheckResult) error {
	if !check.UpdateAvailable {
		return nil
	}
	if check.DownloadURL == "" || check.RemoteSHA256 == "" {
		return fmt.Errorf("incomplete update metadata")
	}
	return stageAndApply(ctx, check.DownloadURL, check.RemoteSHA256)
}

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
