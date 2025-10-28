package connect

import (
	"log"
	"mosona-manager/db"
)

func Init() {
	var servers []int64
	err := db.Db.Select(&servers, "SELECT id FROM servers WHERE allow_monitor = true")
	if err != nil {
		log.Fatalln("Failed to load monitor servers:", err)
	}

	for _, serverId := range servers {
		go func() {
			for {
				if err = StartServer(serverId); err != nil {
					log.Printf("Failed to start monitoring for server %d: %v\n", serverId, err)
				} else {
					break
				}
			}
		}()
	}
}
