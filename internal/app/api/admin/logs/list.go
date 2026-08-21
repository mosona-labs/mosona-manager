package mlogs

import (
	"errors"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/utils"
	"strconv"
	"time"

	"github.com/labstack/echo/v5"
)

func list(c *echo.Context) error {
	legacyPage, _ := strconv.Atoi(c.QueryParam("page"))
	if legacyPage > 1 {
		return c.JSON(400, _type.H{Code: "invalid", Msg: "Offset pagination is no longer supported"})
	}
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))
	if pageSize <= 0 {
		pageSize = 20
	}
	if err := influx.ValidateLogPageSize(pageSize); err != nil {
		return c.JSON(400, _type.H{Code: "invalid", Msg: "Invalid pagination"})
	}

	category := c.QueryParam("category")
	level := c.QueryParam("level")
	email := c.QueryParam("email")
	message := c.QueryParam("message")
	if err := influx.ValidateLogFilters(category, level, message); err != nil {
		return c.JSON(400, _type.H{Code: "invalid", Msg: "Invalid log filter"})
	}
	start, end, err := influx.ParseLogTimeRange(c.QueryParam("start"), c.QueryParam("end"), message, time.Now())
	if err != nil {
		return c.JSON(400, _type.H{Code: "invalid", Msg: "Invalid log time range"})
	}
	cursor := c.QueryParam("cursor")

	ctx := c.Request().Context()

	// User filter
	var uids []int64
	if email != "" {
		var err error
		uids, err = db.GetAdminUserIdsByEmail(ctx, email)
		if err != nil {
			return utils.ErrorHandler(c, err, "Database error")
		}
		if len(uids) == 0 {
			return c.JSON(200, _type.H{
				Code: "ok",
				Msg:  "Success",
				Data: _type.Map{
					"logs":        []_type.Log{},
					"next_cursor": "",
					"has_more":    false,
				},
			})
		}
	}

	page, err := influx.GetLogs(ctx, 0, pageSize, category, level, uids, message, start, end, cursor)
	if err != nil {
		if errors.Is(err, influx.ErrInvalidLogFilter) {
			return c.JSON(400, _type.H{Code: "invalid", Msg: "Invalid log query"})
		}
		return utils.ErrorHandler(c, err, "Database error")
	}

	var userIDs []int64
	for _, logRecord := range page.Logs {
		if logRecord.UserID != 0 {
			userIDs = append(userIDs, logRecord.UserID)
		}
	}
	userMap, err := db.GetUserByIds(userIDs)
	if err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	for i, logRecord := range page.Logs {
		user, ok := userMap[logRecord.UserID]
		if !ok {
			user = _type.User{
				ID:       logRecord.UserID,
				Username: "[Deleted]",
			}
		}
		page.Logs[i].Username = user.Username
		page.Logs[i].Email = user.Email
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: _type.Map{
			"logs":        page.Logs,
			"next_cursor": page.NextCursor,
			"has_more":    page.HasMore,
		},
	})
}
