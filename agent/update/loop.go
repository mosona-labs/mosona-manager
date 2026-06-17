package update

import (
	"context"
	"log"
	"strings"
	"time"

	"mosona-manager/agent/config"
)

const (
	defaultCheckInterval    = 12 * time.Hour
	passiveHubCheckInterval = 3 * time.Hour
	initialCheckDelay       = 2 * time.Minute
)

func StartBackgroundLoop() {
	go func() {
		interval := defaultCheckInterval
		if config.Current.Mode == "passive" && strings.TrimSpace(config.Current.Hub) != "" {
			interval = passiveHubCheckInterval
		}
		time.Sleep(initialCheckDelay)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			runBackgroundCheck()
			<-ticker.C
		}
	}()
}

func runBackgroundCheck() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	res, err := Check(ctx)
	if err != nil {
		log.Printf("auto-update check failed: %v", err)
		return
	}
	if !res.UpdateAvailable {
		return
	}
	log.Printf("auto-update: new binary available (release %s), applying", res.ReleaseTag)
	if err := ApplyIfNeeded(ctx, res); err != nil {
		log.Printf("auto-update apply failed: %v", err)
	}
}
