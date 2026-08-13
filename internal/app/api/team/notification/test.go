package anotification

import (
	"context"
	"errors"
	"sync"
	"time"

	"mosona-manager/internal/_type"
	"mosona-manager/internal/config"
	"mosona-manager/internal/notification"

	"github.com/labstack/echo/v5"
)

const notificationTestInterval = 5 * time.Second

var notificationTestState = struct {
	sync.Mutex
	last   map[int64]time.Time
	active map[int64]bool
	calls  uint64
}{last: make(map[int64]time.Time), active: make(map[int64]bool)}

func test(c *echo.Context) error {
	tid, _ := c.Get("tid").(int64)
	release, ok := beginNotificationTest(tid, time.Now())
	if !ok {
		return c.JSON(429, _type.H{Code: "rate_limited", Msg: "Please wait before testing another notification"})
	}
	defer release()

	uri := c.FormValue("uri")
	if err := notification.Send(c.Request().Context(), uri, "Test notification from Mosona Manager\n\n"+config.ReadDynamicConf().Domain); err != nil {
		if errors.Is(err, notification.ErrInvalidConfiguration) {
			return c.JSON(400, _type.H{Code: "invalid_notification", Msg: err.Error()})
		}
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  notificationTestError(err),
		})
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Test notification sent successfully",
	})
}

func beginNotificationTest(teamID int64, now time.Time) (func(), bool) {
	notificationTestState.Lock()
	defer notificationTestState.Unlock()
	notificationTestState.calls++
	if notificationTestState.calls%256 == 0 {
		for id, last := range notificationTestState.last {
			if !notificationTestState.active[id] && now.Sub(last) >= 2*notificationTestInterval {
				delete(notificationTestState.last, id)
			}
		}
	}
	if teamID <= 0 || notificationTestState.active[teamID] || now.Sub(notificationTestState.last[teamID]) < notificationTestInterval {
		return func() {}, false
	}
	notificationTestState.active[teamID] = true
	notificationTestState.last[teamID] = now
	return func() {
		notificationTestState.Lock()
		delete(notificationTestState.active, teamID)
		notificationTestState.Unlock()
	}, true
}

func notificationTestError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "Notification delivery timed out"
	}
	if errors.Is(err, notification.ErrRateLimited) {
		return "Notification delivery rate limit exceeded"
	}
	if errors.Is(err, notification.ErrInvalidConfiguration) {
		return err.Error()
	}
	return "Notification delivery failed"
}
