package email

import (
	"embed"
	"strings"
)

//go:embed templates/*
var templateFS embed.FS

func replacePlaceholder(body, placeholder, value string) string {
	return strings.ReplaceAll(body, placeholder, value)
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

func GetNotificationTemplate(username, title, description, link string) (string, error) {
	data, err := templateFS.ReadFile("templates/notification.html")
	if err != nil {
		return "", err
	}
	body := string(data)

	body = replacePlaceholder(body, "{{USER_NAME}}", username)
	body = replacePlaceholder(body, "{{TITLE}}", title)
	body = replacePlaceholder(body, "{{DESCRIPTION}}", description)
	body = replacePlaceholder(body, "{{LINK}}", link)

	return body, nil
}
