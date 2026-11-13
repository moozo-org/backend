package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"moozo/moozo"
)

// PasswordResetCodeLifetime is how long an emailed reset code can be used.
const PasswordResetCodeLifetime = 30 * time.Minute

type ForgotPassword struct {
	Email string
}

type ForgotPasswordHandler struct {
	users  moozo.UserRepository
	codes  moozo.PasswordResetCodeRepository
	mailer moozo.Mailer
}

func NewForgotPasswordHandler(users moozo.UserRepository, codes moozo.PasswordResetCodeRepository, mailer moozo.Mailer) ForgotPasswordHandler {
	return ForgotPasswordHandler{users: users, codes: codes, mailer: mailer}
}

// Execute emails the user a reset code, replacing any previous one. It also
// succeeds, sending nothing, for an unknown email or within ResendCooldown of
// the last code, so the response reveals nothing and cannot flood an inbox.
func (h ForgotPasswordHandler) Execute(ctx context.Context, cmd ForgotPassword) (struct{}, error) {
	u, err := h.users.FindUserByEmail(ctx, normalizeEmail(cmd.Email))
	if errors.Is(err, moozo.ErrUserNotFound) {
		return struct{}{}, nil
	}
	if err != nil {
		return struct{}{}, fmt.Errorf("find user: %w", err)
	}

	c, err := h.codes.FindPasswordResetCode(ctx, u.ID)
	switch {
	case err == nil && time.Since(c.CreatedAt) < ResendCooldown:
		return struct{}{}, nil
	case err != nil && !errors.Is(err, moozo.ErrVerificationCodeNotFound):
		return struct{}{}, fmt.Errorf("find reset code: %w", err)
	}

	code, hash := moozo.NewVerificationCode()
	// MongoDB stores dates at millisecond precision
	now := time.Now().UTC().Truncate(time.Millisecond)
	err = h.codes.SavePasswordResetCode(ctx, &moozo.VerificationCode{
		UserID:    u.ID,
		CodeHash:  hash,
		CreatedAt: now,
		ExpiresAt: now.Add(PasswordResetCodeLifetime),
	})
	if err != nil {
		return struct{}{}, fmt.Errorf("save reset code: %w", err)
	}
	if err := h.mailer.SendPasswordResetCode(ctx, u.Email, code); err != nil {
		return struct{}{}, fmt.Errorf("send reset code: %w", err)
	}
	return struct{}{}, nil
}
