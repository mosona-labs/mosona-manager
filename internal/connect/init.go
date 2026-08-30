package connect

import (
	"log"
	"mosona-manager/internal/connect/conn"
	"mosona-manager/internal/db"
)

func Init() {
	rows, err := db.Db.Query("SELECT id FROM servers WHERE (type <> 2 AND allow_monitor = true) OR type = 1")
	if err != nil {
		log.Fatalln("Failed to load monitor servers:", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	semaphore := make(chan struct{}, 5)

	for rows.Next() {
		var serverId int64
		if err = rows.Scan(&serverId); err != nil {
			log.Println("Failed to scan server row:", err)
			continue
		}
		semaphore <- struct{}{}
		go func(id int64) {
			defer func() { <-semaphore }()
			if err := conn.ReconcileServer(id); err != nil {
				log.Printf("Failed initial monitoring reconciliation for server %d: %v", id, err)
			}
		}(serverId)
	}
	if err := rows.Err(); err != nil {
		log.Fatalln("Failed to load monitor servers:", err)
	}
}
