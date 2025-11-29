package task

import "time"

func Init() {
	go runTask(SystemUsage, 10*time.Second) // 10 seconds
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
