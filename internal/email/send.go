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
	dc := config.ReadDynamicConf()
	switch dc.EmailProvider {
	case "smtp":
		return sendWithSMTP(dc, toEmail, subject, content)
	default:
		return ErrNoEmailProvider
	}
}

func VerifyEmailProvider() error {
	dc := config.ReadDynamicConf()

	switch dc.EmailProvider {
	case "smtp":
		if dc.SMTPHost == "" || dc.SMTPPort == 0 || dc.SMTPUsername == "" || dc.SMTPPassword == "" {
			return ErrEmailProviderNotInit
		}
		return nil
	default:
		return ErrNoEmailProvider
	}
}
