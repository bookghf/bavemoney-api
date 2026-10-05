// Package mail sends transactional email (password reset codes).
//
// Production sends through Resend's HTTP API (RESEND_API_KEY, MAIL_FROM).
// Without a key, development logs each message instead, so the reset flow
// can be tried locally without an email account.
package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Message is one plain-text email.
type Message struct {
	To      string
	Subject string
	Text    string
}

// Sender delivers a Message.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// New returns a Resend sender when apiKey is set, and a log sender otherwise.
func New(apiKey, from string) Sender {
	if apiKey == "" {
		return LogSender{}
	}
	return &ResendSender{apiKey: apiKey, from: from, client: &http.Client{Timeout: 10 * time.Second}}
}

// LogSender writes messages to the log instead of sending them. Development
// only: the log then holds reset codes.
type LogSender struct{}

// Send logs msg.
func (LogSender) Send(_ context.Context, msg Message) error {
	slog.Info("mail not sent (no RESEND_API_KEY)", "to", msg.To, "subject", msg.Subject, "text", msg.Text)
	return nil
}

// ResendSender sends through https://resend.com.
type ResendSender struct {
	apiKey string
	from   string
	client *http.Client
}

const resendURL = "https://api.resend.com/emails"

// Send posts msg to Resend.
func (s *ResendSender) Send(ctx context.Context, msg Message) error {
	body, err := json.Marshal(map[string]interface{}{
		"from":    s.from,
		"to":      []string{msg.To},
		"subject": msg.Subject,
		"text":    msg.Text,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, resendURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	res, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return fmt.Errorf("resend: %s: %s", res.Status, detail)
	}
	return nil
}
