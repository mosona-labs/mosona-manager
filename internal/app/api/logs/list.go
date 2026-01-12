package alogs

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/utils"
	"strconv"

	"github.com/labstack/echo/v4"
)

func list(c echo.Context) error {
	tid, _ := c.Get("tid").(int64)

	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 1000 {
		pageSize = 20
	}

	category := c.QueryParam("category")
	level := c.QueryParam("level")
	email := c.QueryParam("email")
	message := c.QueryParam("message")

	// User filter
	var uids []int64
	if email != "" {
		var err error
		uids, err = db.GetTeamUserIdsByEmail(tid, email)
		if err != nil {
			return utils.ErrorHandler(c, err, "Database error")
		}
		if len(uids) == 0 {
			return c.JSON(200, _type.H{
				Code: "ok",
				Msg:  "Success",
				Data: echo.Map{
					"logs":  []_type.Log{},
					"total": 0,
				},
			})
		}
	}

	data, total, err := influx.GetLogsByPage(tid, page, pageSize, category, level, uids, message)
	if err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	var userIDs []int64
	for _, logRecord := range data {
		if logRecord.UserID != 0 {
			userIDs = append(userIDs, logRecord.UserID)
		}
	}
	userMap, err := db.GetUserByIds(userIDs)
	if err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	for i, logRecord := range data {
		user, ok := userMap[logRecord.UserID]
		if !ok {
			user = _type.User{
				ID:       logRecord.UserID,
				Username: "[Deleted]",
			}
		}
		data[i].Username = user.Username
		data[i].Email = user.Email
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: echo.Map{
			"logs":  data,
			"total": total,
		},
	})
}
