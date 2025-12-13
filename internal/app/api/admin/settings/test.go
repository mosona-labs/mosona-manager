package msettings

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/db"
	email2 "mosona-manager/internal/email"

	"github.com/labstack/echo/v4"
)

func testEmail(c echo.Context) error {
	uid, _ := c.Get("uid").(int64)

	user, err := db.GetUserById(uid)
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	emailContent, err := email2.GetActivateTemplate(user.Username, "TEST_ACTIVATION_CODE")
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Failed to generate email content: " + err.Error(),
		})
	}
	if err = email2.Send(user.Email, "Test Email from Mosona Manager", emailContent); err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Failed to send email: " + err.Error(),
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Test email sent successfully",
	})
}
