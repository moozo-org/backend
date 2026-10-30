package mail

import (
	"context"

	"go.uber.org/zap"

	"moozo/moozo"
)

// Log writes emails to the log instead of sending them, for local development
// without a Brevo account. It logs the codes in clear: never use it in
// production.
type Log struct {
	logger *zap.Logger
}

var _ moozo.Mailer = Log{}

func NewLog(logger *zap.Logger) Log {
	return Log{logger: logger}
}

func (l Log) SendVerificationCode(_ context.Context, email, code string) error {
	l.logger.Info("verification code (not emailed: BREVO_API_KEY is unset)",
		zap.String("email", email),
		zap.String("code", code),
	)
	return nil
}
