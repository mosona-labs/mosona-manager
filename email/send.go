package email

import (
	"errors"
	"mosona-manager/config"
)

var (
	ErrNoEmailProvider      = errors.New("no email provider found")
	ErrEmailProviderNotInit = errors.New("email provider not initialized")
)

func Send(toEmail, subject, content string) error {
	switch config.DynamicConf.EmailProvider {
	case "smtp":
		return sendWithSMTP(toEmail, subject, content)
	default:
		return ErrNoEmailProvider
	}
}
