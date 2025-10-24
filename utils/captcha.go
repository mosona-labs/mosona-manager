package utils

import (
	"encoding/json"
	"mosona-manager/config"
	"net/http"
	"net/url"
)

type turnstileResponse struct {
	Success    bool     `json:"success"`
	ErrorCodes []string `json:"error-codes"`
}

func VerifyCaptcha(token, ip string) (bool, error) {
	form := url.Values{}
	form.Set("secret", config.DynamicConf.CaptchaSecret)
	form.Set("response", token)
	form.Set("remoteip", ip)

	resp, err := http.PostForm("https://challenges.cloudflare.com/turnstile/v0/siteverify", form)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	var result turnstileResponse
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}

	return result.Success, nil
}
