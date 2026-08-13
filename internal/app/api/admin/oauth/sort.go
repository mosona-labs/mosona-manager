package moauth

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils"

	"github.com/labstack/echo/v5"
)

func sort(c *echo.Context) error {
	var req = make([]int, 0)
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid request data",
		})
	}

	tx, err := db.Db.Begin()
	if err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}
	defer func() { _ = tx.Rollback() }()

	for index, id := range req {
		_, err = tx.Exec(
			`UPDATE auth_provider SET sort = $1 WHERE id = $2`,
			index, id,
		)
		if err != nil {
			_ = tx.Rollback()
			return utils.ErrorHandler(c, err, "Database error")
		}
	}

	if err = tx.Commit(); err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "OAuth providers sorted successfully",
	})
}
