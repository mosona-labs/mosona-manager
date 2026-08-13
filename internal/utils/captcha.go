package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	captchaTimeout          = 10 * time.Second
	maxCaptchaResponseBytes = 64 << 10
	captchaVerifyURL        = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
)

var captchaHTTPClient = &http.Client{Timeout: captchaTimeout}

type turnstileResponse struct {
	Success    bool     `json:"success"`
	ErrorCodes []string `json:"error-codes"`
}

func VerifyCaptcha(ctx context.Context, secret, token, ip string) (bool, error) {
	return verifyCaptcha(ctx, captchaHTTPClient, captchaVerifyURL, secret, token, ip)
}

func verifyCaptcha(ctx context.Context, client *http.Client, endpoint, secret, token, ip string) (bool, error) {
	form := url.Values{}
	form.Set("secret", secret)
	form.Set("response", token)
	form.Set("remoteip", ip)

	ctx, cancel := context.WithTimeout(ctx, captchaTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return false, fmt.Errorf("captcha service returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCaptchaResponseBytes+1))
	if err != nil {
		return false, err
	}
	if len(body) > maxCaptchaResponseBytes {
		return false, fmt.Errorf("captcha response exceeds %d bytes", maxCaptchaResponseBytes)
	}

	var result turnstileResponse
	if err = json.Unmarshal(body, &result); err != nil {
		return false, err
	}

	return result.Success, nil
}
