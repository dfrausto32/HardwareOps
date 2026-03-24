package mailer

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"mime/quotedprintable"
	"net"
	"net/smtp"
	"net/url"
	"strings"
	"text/template"
	"time"
)

// SMTPConfig holds the configuration for the SMTP mailer.
type SMTPConfig struct {
	Host       string
	Port       int           // default: 587
	User       string
	Password   string
	From       string        // e.g. "HardwareOps <noreply@corp.example.com>"
	TLSMode    string        // "none" | "starttls" | "tls"
	Timeout    time.Duration // default: 10s
	SkipVerify bool          // dev only; blocked by hardened profile
}

// SMTPMailer sends email via SMTP using stdlib only (no third-party mail library).
type SMTPMailer struct {
	cfg SMTPConfig
}

// NewSMTPMailer creates an SMTPMailer with defaults applied.
func NewSMTPMailer(cfg SMTPConfig) *SMTPMailer {
	if cfg.Port == 0 {
		cfg.Port = 587
	}
	if cfg.TLSMode == "" {
		cfg.TLSMode = "starttls"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.From == "" {
		cfg.From = "HardwareOps <noreply@example.com>"
	}
	return &SMTPMailer{cfg: cfg}
}

var resetTmpl = template.Must(template.New("reset").Parse(
	`You (or an admin) requested a password reset for your HardwareOps account.

Click the link below to set a new password. This link expires at {{.ExpiresAt}}.

  {{.Link}}

If you did not request a reset, you can safely ignore this email.
Your password will not change until you use the link above.
`))

var inviteTmpl = template.Must(template.New("invite").Parse(
	`You've been invited to HardwareOps.

Click the link below to set your password and activate your account.
This link expires at {{.ExpiresAt}}.

  {{.Link}}
`))

func (m *SMTPMailer) SendPasswordReset(to, tokenValue string, expiresAt time.Time, appPublicURL string) error {
	link := buildResetLink(appPublicURL, to, tokenValue)
	var body bytes.Buffer
	if err := resetTmpl.Execute(&body, map[string]any{
		"ExpiresAt": expiresAt.UTC().Format(time.RFC1123),
		"Link":      link,
	}); err != nil {
		return fmt.Errorf("render password reset template: %w", err)
	}
	return m.send(to, "HardwareOps: Reset your password", body.String())
}

func (m *SMTPMailer) SendInvite(to, tokenValue string, expiresAt time.Time, appPublicURL string) error {
	link := buildResetLink(appPublicURL, to, tokenValue)
	var body bytes.Buffer
	if err := inviteTmpl.Execute(&body, map[string]any{
		"ExpiresAt": expiresAt.UTC().Format(time.RFC1123),
		"Link":      link,
	}); err != nil {
		return fmt.Errorf("render invite template: %w", err)
	}
	return m.send(to, "You've been invited to HardwareOps", body.String())
}

func buildResetLink(appPublicURL, email, token string) string {
	base := strings.TrimRight(appPublicURL, "/")
	return base + "/?view=reset-token&email=" + url.QueryEscape(email) + "&token=" + url.QueryEscape(token)
}

func (m *SMTPMailer) send(to, subject, body string) error {
	addr := fmt.Sprintf("%s:%d", m.cfg.Host, m.cfg.Port)
	msg := buildMessage(m.cfg.From, to, subject, body)
	switch strings.ToLower(m.cfg.TLSMode) {
	case "tls":
		return m.sendTLS(addr, to, msg)
	case "none":
		return m.sendPlain(addr, to, msg)
	default: // "starttls"
		return m.sendStartTLS(addr, to, msg)
	}
}

func (m *SMTPMailer) sendPlain(addr, to string, msg []byte) error {
	var auth smtp.Auth
	if m.cfg.User != "" {
		auth = smtp.PlainAuth("", m.cfg.User, m.cfg.Password, m.cfg.Host)
	}
	return smtp.SendMail(addr, auth, extractAddr(m.cfg.From), []string{to}, msg)
}

func (m *SMTPMailer) sendStartTLS(addr, to string, msg []byte) error {
	conn, err := net.DialTimeout("tcp", addr, m.cfg.Timeout)
	if err != nil {
		return fmt.Errorf("dial smtp: %w", err)
	}
	c, err := smtp.NewClient(conn, m.cfg.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("smtp client: %w", err)
	}
	defer c.Close()
	tlsCfg := &tls.Config{
		ServerName:         m.cfg.Host,
		InsecureSkipVerify: m.cfg.SkipVerify, //nolint:gosec
	}
	if err := c.StartTLS(tlsCfg); err != nil {
		return fmt.Errorf("starttls: %w", err)
	}
	if m.cfg.User != "" {
		if err := c.Auth(smtp.PlainAuth("", m.cfg.User, m.cfg.Password, m.cfg.Host)); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	return sendViaClient(c, extractAddr(m.cfg.From), to, msg)
}

func (m *SMTPMailer) sendTLS(addr, to string, msg []byte) error {
	tlsCfg := &tls.Config{
		ServerName:         m.cfg.Host,
		InsecureSkipVerify: m.cfg.SkipVerify, //nolint:gosec
	}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: m.cfg.Timeout}, "tcp", addr, tlsCfg)
	if err != nil {
		return fmt.Errorf("tls dial smtp: %w", err)
	}
	c, err := smtp.NewClient(conn, m.cfg.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("smtp client: %w", err)
	}
	defer c.Close()
	if m.cfg.User != "" {
		if err := c.Auth(smtp.PlainAuth("", m.cfg.User, m.cfg.Password, m.cfg.Host)); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	return sendViaClient(c, extractAddr(m.cfg.From), to, msg)
}

func sendViaClient(c *smtp.Client, from, to string, msg []byte) error {
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("MAIL FROM: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("RCPT TO: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close body: %w", err)
	}
	return c.Quit()
}

func buildMessage(from, to, subject, body string) []byte {
	var qpBuf bytes.Buffer
	qw := quotedprintable.NewWriter(&qpBuf)
	_, _ = fmt.Fprint(qw, body)
	_ = qw.Close()

	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: %s\r\n", from)
	fmt.Fprintf(&msg, "To: %s\r\n", to)
	fmt.Fprintf(&msg, "Subject: %s\r\n", subject)
	fmt.Fprintf(&msg, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&msg, "Content-Type: text/plain; charset=UTF-8\r\n")
	fmt.Fprintf(&msg, "Content-Transfer-Encoding: quoted-printable\r\n")
	fmt.Fprintf(&msg, "\r\n")
	_, _ = msg.Write(qpBuf.Bytes())
	return msg.Bytes()
}

// extractAddr pulls the raw email address from "Display Name <addr@example.com>".
func extractAddr(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "<"); i >= 0 {
		if j := strings.LastIndex(s, ">"); j > i {
			return strings.TrimSpace(s[i+1 : j])
		}
	}
	return s
}
