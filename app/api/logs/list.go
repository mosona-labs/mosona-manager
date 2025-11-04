package alogs

import (
	"mosona-manager/_type"
	"mosona-manager/db"
	"mosona-manager/influx"
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

	data, total, err := influx.GetLogsByPage(tid, page, pageSize)
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	var userIDs []int64
	for _, log := range data {
		if log.UserID != 0 {
			userIDs = append(userIDs, log.UserID)
		}
	}
	userMap, err := db.GetUserByIds(userIDs)
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	for i, log := range data {
		user, ok := userMap[log.UserID]
		if !ok {
			user = _type.User{
				ID:       log.UserID,
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
