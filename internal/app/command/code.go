package command

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"moozo/moozo"
)

// codeAttempts checks emailed codes against a store of them; verification and
// password-reset codes each have their own.
type codeAttempts struct {
	consume func(context.Context, uuid.UUID) (*moozo.VerificationCode, error)
	remove  func(context.Context, uuid.UUID) error
}

// check spends one attempt on the user's code and reports whether code
// matches it. The attempt is counted before comparing, so concurrent guesses
// cannot exceed MaxVerificationAttempts. A code that can no longer succeed is
// deleted. No code at all is a mismatch, not an error.
func (a codeAttempts) check(ctx context.Context, userID uuid.UUID, code string) (bool, error) {
	c, err := a.consume(ctx, userID)
	if errors.Is(err, moozo.ErrVerificationCodeNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("consume code attempt: %w", err)
	}

	hash := moozo.HashVerificationCode(code)
	matches := subtle.ConstantTimeCompare(hash[:], c.CodeHash[:]) == 1
	expired := !time.Now().Before(c.ExpiresAt)
	if matches && !expired && c.Attempts <= MaxVerificationAttempts {
		return true, nil
	}

	if expired || c.Attempts >= MaxVerificationAttempts {
		if err := a.remove(ctx, userID); err != nil {
			return false, fmt.Errorf("delete code: %w", err)
		}
	}
	return false, nil
}
