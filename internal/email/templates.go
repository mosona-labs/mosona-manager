package email

import (
	"embed"
	"strings"
)

//go:embed templates/*
var templateFS embed.FS

func replacePlaceholder(body, placeholder, value string) string {
	return strings.Replace(body, placeholder, value, -1)
}

func GetActivateTemplate(username, code string) (string, error) {
	data, err := templateFS.ReadFile("templates/activate.html")
	if err != nil {
		return "", err
	}
	body := string(data)

	body = replacePlaceholder(body, "{{USER_NAME}}", username)
	body = replacePlaceholder(body, "{{CODE}}", code)

	return body, nil
}

func GetTwoFATemplate(username, code string) (string, error) {
	data, err := templateFS.ReadFile("templates/2fa.html")
	if err != nil {
		return "", err
	}
	body := string(data)

	body = replacePlaceholder(body, "{{USER_NAME}}", username)
	body = replacePlaceholder(body, "{{CODE}}", code)

	return body, nil
}
