package moozo

import (
	"context"

	"github.com/google/uuid"
)

// PasswordResetCodeRepository stores the codes emailed by forgot-password.
// They are VerificationCodes kept apart from the email-verification ones, so
// asking for a reset does not replace a pending user's verification code. A
// user has at most one; issuing a new one replaces it.
type PasswordResetCodeRepository interface {
	// SavePasswordResetCode stores c, replacing the user's previous code.
	SavePasswordResetCode(ctx context.Context, c *VerificationCode) error
	// FindPasswordResetCode returns ErrVerificationCodeNotFound when the user
	// has no code. It may return an expired code; callers check ExpiresAt.
	FindPasswordResetCode(ctx context.Context, userID uuid.UUID) (*VerificationCode, error)
	// ConsumePasswordResetAttempt atomically increments the code's Attempts
	// and returns the updated code. It returns ErrVerificationCodeNotFound
	// when there is no code.
	ConsumePasswordResetAttempt(ctx context.Context, userID uuid.UUID) (*VerificationCode, error)
	// DeletePasswordResetCode reports whether there was a code to delete, so
	// of two concurrent resets with the same code only one goes through.
	DeletePasswordResetCode(ctx context.Context, userID uuid.UUID) (bool, error)
}
