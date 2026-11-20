package command

import (
	"context"
	"fmt"
	"time"

	"moozo/moozo"
)

// VerificationCodeLifetime is how long an emailed code can be used.
const VerificationCodeLifetime = 15 * time.Minute

// sendVerificationCode issues a new code to a user, replacing any previous
// one, and emails it. It is shared by RegisterUser and ResendVerification.
type sendVerificationCode struct {
	codes  moozo.VerificationCodeRepository
	mailer moozo.Mailer
}

func (s sendVerificationCode) send(ctx context.Context, u *moozo.User) error {
	code, hash := moozo.NewVerificationCode()
	// MongoDB stores dates at millisecond precision
	now := time.Now().UTC().Truncate(time.Millisecond)
	c := &moozo.VerificationCode{
		UserID:    u.ID,
		CodeHash:  hash,
		CreatedAt: now,
		ExpiresAt: now.Add(VerificationCodeLifetime),
	}
	if err := s.codes.SaveVerificationCode(ctx, c); err != nil {
		return fmt.Errorf("save verification code: %w", err)
	}
	if err := s.mailer.SendVerificationCode(ctx, u.Email, code); err != nil {
		return fmt.Errorf("send verification code: %w", err)
	}
	return nil
}
