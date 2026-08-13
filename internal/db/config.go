package db

import (
	"errors"
	"mosona-manager/internal/config"
	"mosona-manager/internal/utils"
	"strconv"
	"sync"
)

type configType struct {
	Key   string
	Value string
}

var syncConfigMu sync.Mutex
var generateConfigToken = utils.RandomString

func SyncConfig() error {
	syncConfigMu.Lock()
	defer syncConfigMu.Unlock()

	var data []configType

	if err := Db.Select(&data, "SELECT key, value FROM config"); err != nil {
		return err
	}

	next := config.ReadDynamicConf()

	for _, item := range data {
		switch item.Key {
		case "init":
			next.Init = item.Value == "true"
		case "debug":
			next.Debug = item.Value == "true"
		case "title":
			next.Title = item.Value
		case "favicon":
			next.Favicon = item.Value
		case "domain":
			next.Domain = item.Value
		case "token":
			next.Token = item.Value
		case "captcha_secret":
			next.CaptchaSecret = item.Value
		case "captcha_site_key":
			next.CaptchaSiteKey = item.Value
		case "email_verify_login":
			next.EmailVerifyLogin = item.Value == "true"
		case "email_provider":
			next.EmailProvider = item.Value
		case "smtp_host":
			next.SMTPHost = item.Value
		case "smtp_port":
			next.SMTPPort, _ = strconv.Atoi(item.Value)
		case "smtp_username":
			next.SMTPUsername = item.Value
		case "smtp_password":
			next.SMTPPassword = item.Value
		case "smtp_tls":
			next.SMTPTls = item.Value == "true"
		case "registration_enabled":
			next.RegistrationEnabled = item.Value == "true" // Default false
		case "registration_verify_email":
			next.RegistrationVerifyEmail = item.Value == "true" // Default false
		case "session_bind_ip":
			next.SessionBindIP = item.Value != "false" // Default true
		case "trust_proxy":
			next.TrustProxy = item.Value != "false" // Default true
		}
	}

	// Init
	if next.Token == "" {
		token := generateConfigToken(32)
		if len(token) != 32 {
			return errors.New("failed to generate configuration token")
		}
		if err := SetConfig("token", token); err != nil {
			return err
		}
		next.Token = token
	}

	config.ReplaceDynamicConf(next)
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
