package update

import (
	"context"
	"log"
	"time"
)

const (
	defaultCheckInterval = 12 * time.Hour
	initialCheckDelay    = 2 * time.Minute
)

func StartBackgroundLoop() {
	go func() {
		time.Sleep(initialCheckDelay)
		ticker := time.NewTicker(defaultCheckInterval)
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
