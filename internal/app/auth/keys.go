package auth

import (
	"mosona-manager/internal/config"
	"mosona-manager/internal/db"
	_type2 "mosona-manager/pkg/_type"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

var (
	cachedOAuthList []_type2.PublicAuthProvider
	cacheTime       int64
	cacheTTL        = 30 // seconds
	cacheMutex      sync.RWMutex
)

func getOAuthListCached() ([]_type2.PublicAuthProvider, error) {
	cacheMutex.RLock()
	if time.Now().Unix()-cacheTime < int64(cacheTTL) && cachedOAuthList == nil {
		cacheMutex.RUnlock()
		return cachedOAuthList, nil
	}
	cacheMutex.RUnlock()

	list, err := db.GetOAuthList()
	if err != nil {
		return nil, err
	}

	cacheMutex.Lock()
	cachedOAuthList = list
	cacheTime = time.Now().Unix()
	cacheMutex.Unlock()

	return list, nil
}

func keys(c echo.Context) error {
	oauthList, err := getOAuthListCached()
	if err != nil {
		return c.JSON(500, _type2.H{Code: "error", Msg: "Database Error"})
	}

	return c.JSON(200, _type2.H{
		Code: "ok",
		Msg:  "Success",
		Data: echo.Map{
			"captcha": config.DynamicConf.CaptchaSiteKey,
			"oauth":   oauthList,
		},
	})
}
