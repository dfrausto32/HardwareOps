package mailer

import "time"

// Mailer is the interface for sending email notifications.
type Mailer interface {
	SendPasswordReset(to, tokenValue string, expiresAt time.Time, appPublicURL string) error
	SendInvite(to, tokenValue string, expiresAt time.Time, appPublicURL string) error
}

// NoopMailer is used when SMTP is not configured. All methods return nil silently.
type NoopMailer struct{}

func (n *NoopMailer) SendPasswordReset(to, tokenValue string, expiresAt time.Time, appPublicURL string) error {
	return nil
}

func (n *NoopMailer) SendInvite(to, tokenValue string, expiresAt time.Time, appPublicURL string) error {
	return nil
}
