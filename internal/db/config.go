package db

import (
	"mosona-manager/internal/config"
	"mosona-manager/internal/utils"
	"strconv"
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

	config.DConfLock.Lock()
	defer config.DConfLock.Unlock()

	for _, item := range data {
		switch item.Key {
		case "init":
			config.DynamicConf.Init = item.Value == "true"
		case "debug":
			config.DynamicConf.Debug = item.Value == "true"
		case "title":
			config.DynamicConf.Title = item.Value
		case "favicon":
			config.DynamicConf.Favicon = item.Value
		case "domain":
			config.DynamicConf.Domain = item.Value
		case "token":
			config.DynamicConf.Token = item.Value
		case "captcha_secret":
			config.DynamicConf.CaptchaSecret = item.Value
		case "captcha_site_key":
			config.DynamicConf.CaptchaSiteKey = item.Value
		case "email_verify_login":
			config.DynamicConf.EmailVerifyLogin = item.Value == "true"
		case "email_provider":
			config.DynamicConf.EmailProvider = item.Value
		case "smtp_host":
			config.DynamicConf.SMTPHost = item.Value
		case "smtp_port":
			config.DynamicConf.SMTPPort, _ = strconv.Atoi(item.Value)
		case "smtp_username":
			config.DynamicConf.SMTPUsername = item.Value
		case "smtp_password":
			config.DynamicConf.SMTPPassword = item.Value
		case "smtp_tls":
			config.DynamicConf.SMTPTls = item.Value == "true"
		case "registration_enabled":
			config.DynamicConf.RegistrationEnabled = item.Value == "true" // Default false
		case "registration_verify_email":
			config.DynamicConf.RegistrationVerifyEmail = item.Value == "true" // Default false
		case "session_bind_ip":
			config.DynamicConf.SessionBindIP = item.Value == "true"
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
