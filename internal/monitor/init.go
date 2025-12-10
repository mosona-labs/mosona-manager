package monitor

import (
	"log"
	"mosona-manager/internal/db"
	"mosona-manager/pkg/_type"
	"sync"
)

var (
	mu    sync.Mutex
	tasks []_type.ServerConnect
)

func Init() {
	Refresh()

	go func() {}()
}

func Refresh() {
	mu.Lock()
	defer mu.Unlock()

	if err := db.Db.Select(
		&tasks,
		"SELECT id, team_id, name, address, port, username, password FROM servers WHERE allow_monitor = true",
	); err != nil {
		log.Fatalln("Failed to load monitor tasks:", err)
	}
}

func AddTask(task _type.ServerConnect) {
	mu.Lock()
	defer mu.Unlock()
	tasks = append(tasks, task)
}
