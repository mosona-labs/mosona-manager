package email

import (
	"crypto/tls"
	"fmt"
	"mosona-manager/config"
	"net/smtp"
)

func sendWithSMTP(toEmail, subject, content string) error {
	if config.DynamicConf.SMTPHost == "" || config.DynamicConf.SMTPPort == 0 || config.DynamicConf.SMTPUsername == "" || config.DynamicConf.SMTPPassword == "" {
		return ErrEmailProviderNotInit
	}

	// Setup SMTP client
	auth := smtp.PlainAuth("", config.DynamicConf.SMTPUsername, config.DynamicConf.SMTPPassword, config.DynamicConf.SMTPHost)
	addr := fmt.Sprintf("%s:%d", config.DynamicConf.SMTPHost, config.DynamicConf.SMTPPort)
	from := config.DynamicConf.SMTPUsername
	tlsConf := &tls.Config{
		InsecureSkipVerify: false,
		ServerName:         config.DynamicConf.SMTPHost,
	}
	useTLS := config.DynamicConf.SMTPTls

	// Email headers
	header := make(map[string]string)
	header["From"] = from
	header["To"] = toEmail
	header["Subject"] = subject
	header["MIME-Version"] = "1.0"
	header["Content-Type"] = "text/html; charset=UTF-8"

	message := ""
	for k, v := range header {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n" + content

	client, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("connect to SMTP: %v", err)
	}
	defer func(client *smtp.Client) {
		_ = client.Close()
	}(client)

	if useTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err = client.StartTLS(tlsConf); err != nil {
				return fmt.Errorf("TLS: %v", err)
			}
		} else {
			return fmt.Errorf("STARTTLS does not supported")
		}
	}

	if err = client.Auth(auth); err != nil {
		return err
	}
	if err = client.Mail(from); err != nil {
		return err
	}
	if err = client.Rcpt(toEmail); err != nil {
		return err
	}

	writer, err := client.Data()
	if err != nil {
		return err
	}

	_, err = writer.Write([]byte(message))
	if err != nil {
		return err
	}

	err = writer.Close()
	if err != nil {
		return err
	}

	return client.Quit()
}
