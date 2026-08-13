package task

import (
	"log"
	"mosona-manager/internal/db"
	"time"
)

type serverExpireInfo struct {
	ID      int64     `db:"sid"`
	EndTime time.Time `db:"end_time"`
	Cycle   int       `db:"cycle"` // 1 - monthly, 2 - quarterly, 3 - semiannually, 4 - annually
}

func AutoRenew() {
	var data []serverExpireInfo
	if err := db.Db.Select(&data,
		"SELECT sid, end_time, cycle FROM server_info WHERE end_time IS NOT NULL AND end_time < NOW() AND auto_renew AND cycle > 0",
	); err != nil {
		log.Println("auto_renew:", err)
		return
	}

	tx, err := db.Db.Begin()
	if err != nil {
		log.Println("auto_renew:", err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	for _, v := range data {
		var newEndTime time.Time
		switch v.Cycle {
		case 1:
			newEndTime = v.EndTime.AddDate(0, 1, 0)
		case 2:
			newEndTime = v.EndTime.AddDate(0, 3, 0)
		case 3:
			newEndTime = v.EndTime.AddDate(0, 6, 0)
		case 4:
			newEndTime = v.EndTime.AddDate(1, 0, 0)
		default:
			continue
		}

		if _, err = tx.Exec(
			"UPDATE server_info SET end_time=$1 WHERE sid=$2",
			newEndTime, v.ID,
		); err != nil {
			log.Println("auto_renew:", err)
			_ = tx.Rollback()
			return
		}
	}
	if err = tx.Commit(); err != nil {
		log.Println("auto_renew:", err)
	}
}
