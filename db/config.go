package db

import (
	"mosona-manager/config"
	"mosona-manager/utils"
)

type configType struct {
	Key   string
	Value string
}

func SyncConfig() error {
	var data []configType

	if err := Db.Select(&data, "SELECT key, value FROM config"); err != nil {
		return err
	}
	for _, item := range data {
		switch item.Key {
		case "init":
			config.DynamicConf.Init = item.Value == "true"
		case "token":
			config.DynamicConf.Token = item.Value
		case "captcha_secret":
			config.DynamicConf.CaptchaSecret = item.Value
		case "captcha_site_key":
			config.DynamicConf.CaptchaSiteKey = item.Value
		case "google_client_id":
			config.DynamicConf.GoogleClientID = item.Value
		case "google_client_secret":
			config.DynamicConf.GoogleClientSecret = item.Value
		}
	}

	// Init
	if config.DynamicConf.Token == "" {
		token := utils.RandomString(32)
		if err := SetConfig("token", token); err != nil {
			return err
		}
		config.DynamicConf.Token = token
	}

	return nil
}

func SetConfig(key, value string) error {
	_, err := Db.Exec(
		`INSERT INTO config (key, value)
		 VALUES ($1, $2)
		 ON CONFLICT (key) 
		 DO UPDATE SET 
		 	value = EXCLUDED.value`,
		key, value,
	)
	return err
}
