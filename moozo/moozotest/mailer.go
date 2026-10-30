package moozotest

import (
	"context"
	"sync"

	"moozo/moozo"
)

// SentCode is a verification code a Mailer was asked to send.
type SentCode struct {
	Email string
	Code  string
}

// Mailer is a moozo.Mailer that records what it sends. The zero value is
// ready to use.
type Mailer struct {
	// Err, when set, is returned by every method, to simulate an outage.
	Err error

	mu   sync.Mutex
	sent []SentCode
}

var _ moozo.Mailer = (*Mailer)(nil)

func (m *Mailer) SendVerificationCode(_ context.Context, email, code string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	m.sent = append(m.sent, SentCode{Email: email, Code: code})
	return nil
}

// Sent returns the codes sent, oldest first.
func (m *Mailer) Sent() []SentCode {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]SentCode(nil), m.sent...)
}
