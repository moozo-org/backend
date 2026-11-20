package command

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"time"

	"moozo/moozo"
)

// MaxVerificationAttempts is how many codes can be tried against one emailed
// code; after that the user must ask for a new one.
const MaxVerificationAttempts = 5

// ErrInvalidVerificationCode covers an unknown email, a wrong, expired or
// exhausted code, and no code at all, so callers cannot tell them apart.
var ErrInvalidVerificationCode = errors.New("invalid or expired verification code")

type VerifyEmail struct {
	Email string
	Code  string
}

type VerifyEmailHandler struct {
	users moozo.UserRepository
	codes moozo.VerificationCodeRepository
}

func NewVerifyEmailHandler(users moozo.UserRepository, codes moozo.VerificationCodeRepository) VerifyEmailHandler {
	return VerifyEmailHandler{users: users, codes: codes}
}

// Execute activates the user when the code matches. Verifying an account that
// is already active succeeds, so a retried request is harmless; register
// already reveals which emails have accounts.
func (h VerifyEmailHandler) Execute(ctx context.Context, cmd VerifyEmail) (struct{}, error) {
	u, err := h.users.FindUserByEmail(ctx, normalizeEmail(cmd.Email))
	if errors.Is(err, moozo.ErrUserNotFound) {
		return struct{}{}, ErrInvalidVerificationCode
	}
	if err != nil {
		return struct{}{}, fmt.Errorf("find user: %w", err)
	}
	if u.Status == moozo.StatusActive {
		return struct{}{}, nil
	}

	if err := h.checkCode(ctx, u, cmd.Code); err != nil {
		return struct{}{}, err
	}

	// MongoDB stores dates at millisecond precision
	now := time.Now().UTC().Truncate(time.Millisecond)
	if err := h.users.ActivateUser(ctx, u.ID, now); err != nil {
		return struct{}{}, fmt.Errorf("activate user: %w", err)
	}
	if err := h.codes.DeleteVerificationCode(ctx, u.ID); err != nil {
		return struct{}{}, fmt.Errorf("delete verification code: %w", err)
	}
	return struct{}{}, nil
}

// checkCode spends one attempt on u's code and compares it with code. The
// attempt is counted before comparing, so concurrent guesses cannot exceed
// MaxVerificationAttempts. A code that can no longer succeed is deleted.
func (h VerifyEmailHandler) checkCode(ctx context.Context, u *moozo.User, code string) error {
	c, err := h.codes.ConsumeVerificationAttempt(ctx, u.ID)
	if errors.Is(err, moozo.ErrVerificationCodeNotFound) {
		return ErrInvalidVerificationCode
	}
	if err != nil {
		return fmt.Errorf("consume verification attempt: %w", err)
	}

	hash := moozo.HashVerificationCode(code)
	matches := subtle.ConstantTimeCompare(hash[:], c.CodeHash[:]) == 1
	expired := !time.Now().Before(c.ExpiresAt)
	if matches && !expired && c.Attempts <= MaxVerificationAttempts {
		return nil
	}

	if expired || c.Attempts >= MaxVerificationAttempts {
		if err := h.codes.DeleteVerificationCode(ctx, u.ID); err != nil {
			return fmt.Errorf("delete verification code: %w", err)
		}
	}
	return ErrInvalidVerificationCode
}
