package connect

import (
	"log"
	"mosona-manager/internal/db"
)

func Init() {
	initSSH()
}

func initSSH() {
	var servers []int64
	err := db.Db.Select(&servers, "SELECT id FROM servers WHERE type = 0 AND allow_monitor = true")
	if err != nil {
		log.Fatalln("Failed to load monitor servers:", err)
	}

	semaphore := make(chan struct{}, 5)

	for _, serverId := range servers {
		semaphore <- struct{}{}
		go func(id int64) {
			defer func() { <-semaphore }()
			for {
				if err := StartServer(id); err != nil {
					log.Printf("Failed to start monitoring for server %d: %v\n", id, err)
				} else {
					break
				}
			}
		}(serverId)
	}
}
