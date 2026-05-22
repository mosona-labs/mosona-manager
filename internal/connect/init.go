package connect

import (
	"log"
	"mosona-manager/internal/connect/conn"
	"mosona-manager/internal/db"
)

func Init() {
	rows, err := db.Db.Query("SELECT id, type FROM servers WHERE type <> 2 AND allow_monitor = true")
	if err != nil {
		log.Fatalln("Failed to load monitor servers:", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	semaphore := make(chan struct{}, 5)

	for rows.Next() {
		var serverId int64
		var serverType int16
		if err = rows.Scan(&serverId, &serverType); err != nil {
			log.Println("Failed to scan server row:", err)
			continue
		}
		semaphore <- struct{}{}
		go func(id int64, typ int16) {
			defer func() { <-semaphore }()
			for {
				if err := conn.StartServer(id, typ); err != nil {
					log.Printf("Failed to start monitoring for server %d: %v\n", id, err)
				} else {
					break
				}
			}
		}(serverId, serverType)
	}
}
