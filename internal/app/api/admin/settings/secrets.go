package msettings

const secretMask = "********"

var sensitiveSettingKeys = map[string]struct{}{
	"captcha_secret": {},
	"smtp_password":  {},
}

func isSensitiveSetting(key string) bool {
	_, ok := sensitiveSettingKeys[key]
	return ok
}

func maskSecret(value string) string {
	if value == "" {
		return ""
	}
	return secretMask
}
