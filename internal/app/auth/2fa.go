package auth

import (
	"database/sql"
	"errors"
	"fmt"
	"mosona-manager/internal/config"
	db2 "mosona-manager/internal/db"
	email2 "mosona-manager/internal/email"
	"mosona-manager/pkg/_type"
	"mosona-manager/pkg/utils"
	store2 "mosona-manager/pkg/utils/store"
	"strconv"
	"time"

	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"github.com/pquerna/otp/totp"
)

func middlewareFA(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		sess, err := session.Get("session", c)
		if err != nil {
			return c.JSON(500, _type.H{Code: "error", Msg: "Session error"})
		}

		// 2FA Required
		pUID := sess.Values["pre_2fa_uid"]
		if pUID != nil {
			c.Set("pre_2fa_uid", pUID)
		} else {
			return c.JSON(400, _type.H{
				Code: "error",
				Msg:  "Two-factor authentication not required",
			})
		}

		return next(c)
	}
}

func getTwoFAStatus(c echo.Context) error {
	preUID, ok := c.Get("pre_2fa_uid").(int64)
	if !ok || preUID <= 0 {
		return c.JSON(400, _type.H{
			Code: "unauthorized",
			Msg:  "You are not authorized for 2FA",
		})
	}

	user, err := db2.GetUserById(preUID)
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error: " + err.Error(),
		})
	}

	cooling, ok := store2.GetCooling(preUID)
	if !ok {
		cooling = 0
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: echo.Map{
			"verified":  user.Verified,
			"totp":      user.TOTP,
			"login_2fa": config.DynamicConf.EmailVerifyLogin,
			"cooling":   cooling - time.Now().Unix(),
		},
	})
}

func sendMFACode(c echo.Context) error {
	preUID, ok := c.Get("pre_2fa_uid").(int64)
	if !ok || preUID <= 0 {
		return c.JSON(400, _type.H{
			Code: "unauthorized",
			Msg:  "You are not authorized for 2FA",
		})
	}

	mode := c.FormValue("mode")

	// User
	user, err := db2.GetUserById(preUID)
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error: " + err.Error(),
		})
	}

	if cooling, ok := store2.GetCooling(preUID); ok && cooling > time.Now().Unix() {
		return c.JSON(429, _type.H{
			Code: "cooling",
			Msg:  fmt.Sprintf("Please wait %d seconds before requesting another code", cooling-time.Now().Unix()),
		})
	}

	// New code
	code := strconv.Itoa(utils.RandomNumber(100000, 999999))

	// Email
	var content string
	switch mode {
	case "activation":
		content, err = email2.GetActivateTemplate(user.Username, fmt.Sprintf("%s-%s", code[0:3], code[3:6]))
	case "2fa":
		content, err = email2.GetTwoFATemplate(user.Username, fmt.Sprintf("%s-%s", code[0:3], code[3:6]))
	default:
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid mode",
		})
	}
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Failed to generate email content: " + err.Error(),
		})
	}
	if err = email2.Send(user.Email, "Activate Your Account", content); err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Failed to send email: " + err.Error(),
		})
	}

	// Store code
	store2.SetTwoFACodeState(code, preUID, time.Now().Unix()+60*60)

	// Set cooling period of 60 seconds
	store2.SetCooling(preUID, time.Now().Unix()+60)

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "MFA code sent successfully",
	})
}

func verifyMFACode(c echo.Context) error {
	uid, _ := c.Get("pre_2fa_uid").(int64)
	code := c.FormValue("code")
	if code == "" {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "MFA code cannot be empty",
		})
	}

	storedData, ok := store2.GetTwoFACodeState(code)
	if !ok || storedData.UID != uid {
		store2.DeleteTwoFACodeByUID(uid)

		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid MFA code",
		})
	}

	// Delete used code
	store2.DeleteTwoFACodeState(code)

	user, err := db2.GetUserById(uid)
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}
	if user.Verified != nil && !*user.Verified {
		if _, err = db2.Db.Exec("UPDATE users SET verified = true WHERE id = $1", uid); err != nil {
			return c.JSON(500, _type.H{
				Code: "error",
				Msg:  "Database error",
			})
		}
	}

	// Session
	sess, err := session.Get("session", c)
	if err != nil {
		return c.JSON(500, _type.H{Code: "error", Msg: "Session init error"})
	}

	// Active Team
	activeTid, err := db2.GetUserActiveTeam(user.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return c.JSON(500, _type.H{Code: "error", Msg: "Database error"})
	}
	sess.Values["uid"] = user.ID
	sess.Values["tid"] = activeTid
	sess.Values["user_agent"] = c.Request().Header.Get("User-Agent")
	sess.Values["time"] = time.Now().Unix()
	delete(sess.Values, "pre_2fa_uid")
	if err = sess.Save(c.Request(), c.Response()); err != nil {
		return c.JSON(500, _type.H{Code: "error", Msg: "Session save failed"})
	}

	loginEvent(user.ID, sess.ID, c.RealIP(), c.Request().Header.Get("User-Agent"), user.IsAdmin)

	return c.JSON(200, _type.H{Code: "ok", Msg: "2FA verification successful"})
}

func verifyTOTP(c echo.Context) error {
	uid, _ := c.Get("pre_2fa_uid").(int64)
	totpCode := c.FormValue("code")
	if totpCode == "" {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "TOTP code cannot be empty",
		})
	}

	user, err := db2.GetUserAuthById(uid)
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}

	if user.TOTP == nil || *user.TOTP == "" {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "TOTP is not enabled for this user",
		})
	}

	// Validate TOTP code
	if !totp.Validate(totpCode, *user.TOTP) {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid TOTP code",
		})
	}

	// Session
	sess, err := session.Get("session", c)
	if err != nil {
		return c.JSON(500, _type.H{Code: "error", Msg: "Session init error"})
	}

	// Active Team
	activeTid, err := db2.GetUserActiveTeam(user.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return c.JSON(500, _type.H{Code: "error", Msg: "Database error"})
	}
	sess.Values["uid"] = user.ID
	sess.Values["tid"] = activeTid
	sess.Values["user_agent"] = c.Request().Header.Get("User-Agent")
	sess.Values["time"] = time.Now().Unix()
	delete(sess.Values, "pre_2fa_uid")
	if err = sess.Save(c.Request(), c.Response()); err != nil {
		return c.JSON(500, _type.H{Code: "error", Msg: "Session save failed"})
	}

	loginEvent(user.ID, sess.ID, c.RealIP(), c.Request().Header.Get("User-Agent"), user.IsAdmin)

	return c.JSON(200, _type.H{Code: "ok", Msg: "TOTP verification successful"})
}
