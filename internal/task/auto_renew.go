package task

import (
	"fmt"
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
	if err := autoRenewAt(time.Now()); err != nil {
		log.Println("auto_renew:", err)
	}
}

func autoRenewAt(now time.Time) error {
	tx, err := db.Db.Beginx()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var data []serverExpireInfo
	if err := tx.Select(&data, `
SELECT sid, end_time, cycle
FROM server_info
WHERE end_time IS NOT NULL
  AND end_time < $1
  AND auto_renew
  AND cycle BETWEEN 1 AND 4
FOR UPDATE`, now); err != nil {
		return err
	}

	for _, v := range data {
		newEndTime, ok := nextRenewalEndTime(v.EndTime, now, v.Cycle)
		if !ok {
			return fmt.Errorf("unsupported renewal cycle %d for server %d", v.Cycle, v.ID)
		}

		if _, err := tx.Exec(
			"UPDATE server_info SET end_time=$1 WHERE sid=$2",
			newEndTime, v.ID,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func nextRenewalEndTime(endTime, now time.Time, cycle int) (time.Time, bool) {
	monthsPerCycle := 0
	switch cycle {
	case 1:
		monthsPerCycle = 1
	case 2:
		monthsPerCycle = 3
	case 3:
		monthsPerCycle = 6
	case 4:
		monthsPerCycle = 12
	default:
		return time.Time{}, false
	}

	localNow := now.In(endTime.Location())
	elapsedMonths := (localNow.Year()-endTime.Year())*12 + int(localNow.Month()-endTime.Month())
	cycles := elapsedMonths / monthsPerCycle
	if cycles < 1 {
		cycles = 1
	}

	newEndTime := addAnchoredMonths(endTime, cycles*monthsPerCycle)
	if newEndTime.Before(now) {
		newEndTime = addAnchoredMonths(endTime, (cycles+1)*monthsPerCycle)
	}
	return newEndTime, true
}

// addAnchoredMonths keeps an existing month-end expiration at month end.
// Other dates retain their day when possible and clamp only in shorter months.
func addAnchoredMonths(value time.Time, months int) time.Time {
	monthIndex := value.Year()*12 + int(value.Month()) - 1 + months
	year := monthIndex / 12
	month := time.Month(monthIndex%12 + 1)
	lastDay := daysInMonth(year, month, value.Location())
	day := value.Day()
	if day == daysInMonth(value.Year(), value.Month(), value.Location()) || day > lastDay {
		day = lastDay
	}

	return time.Date(
		year, month, day,
		value.Hour(), value.Minute(), value.Second(), value.Nanosecond(),
		value.Location(),
	)
}

func daysInMonth(year int, month time.Month, location *time.Location) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, location).Day()
}
