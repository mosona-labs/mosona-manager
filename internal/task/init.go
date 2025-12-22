package task

import (
	alerttasks "mosona-manager/internal/task/alerts"
	"time"
)

func Init() {
	go runTask(SystemUsage, 10*time.Second)   // 10 seconds
	go runTask(AutoRenew, 1*time.Hour)        // 1 hour
	go runTask(alerttasks.Run, 5*time.Minute) // 5 minutes
}

func runTask(fn func(), duration time.Duration) {
	ticker := time.NewTicker(duration)
	defer ticker.Stop()

	fn()

	for {
		select {
		case <-ticker.C:
			fn()
		}
	}
}
