package auth

import (
	"context"
	"log"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/redis"
)

func loginEvent(
	uid int64, sessionId string,
	ip string, ua string,
	isAdmin bool,
) {
	// Update login time & Save session ID
	go func() {
		if _, err := db.Db.Exec("UPDATE users SET login_at=NOW() WHERE id=?", uid); err != nil {
			log.Println(err)
		}
		if err := redis.AddSessionID(context.Background(), uid, sessionId); err != nil {
			log.Println(err)
		}
	}()
	// Logs
	go func(ip, ua string) {
		var teamIDs []int64
		rows, err := db.Db.Query("SELECT team_id FROM m_team_user WHERE user_id=?", uid)
		if err != nil {
			log.Println(err)
			return
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var teamID int64
			if err = rows.Scan(&teamID); err != nil {
				log.Println(err)
				return
			}
			teamIDs = append(teamIDs, teamID)
		}

		for _, teamID := range teamIDs {
			influx.LogAdd(
				teamID,
				uid,
				"login",
				"User logged in",
				ip,
				ua,
				"medium",
			)
		}
		if isAdmin {
			influx.LogAdd(
				0,
				uid,
				"login",
				"Admin logged in",
				ip,
				ua,
				"medium",
			)
		}
	}(ip, ua)
}
