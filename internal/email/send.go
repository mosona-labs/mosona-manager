package email

import (
	"errors"
	"mosona-manager/internal/config"
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

func VerifyEmailProvider() error {
	switch config.DynamicConf.EmailProvider {
	case "smtp":
		if config.DynamicConf.SMTPHost == "" || config.DynamicConf.SMTPPort == 0 || config.DynamicConf.SMTPUsername == "" || config.DynamicConf.SMTPPassword == "" {
			return ErrEmailProviderNotInit
		}
		return nil
	default:
		return ErrNoEmailProvider
	}
}
