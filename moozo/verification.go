package moozo

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
)

// VerificationCodeHash is what is stored of a code. Six digits are trivial to
// brute-force from a hash, so this only keeps codes out of plain sight; the
// attempt limit is what protects them.
type VerificationCodeHash [sha256.Size]byte

// VerificationCode is the code a pending user must send back to prove they own
// their email. A user has at most one; issuing a new one replaces it.
type VerificationCode struct {
	UserID   uuid.UUID
	CodeHash VerificationCodeHash
	// Attempts counts the codes checked against this one, the right one
	// included.
	Attempts  int
	CreatedAt time.Time
	ExpiresAt time.Time
}

// NewVerificationCode returns a random six-digit code and the hash to store.
func NewVerificationCode() (string, VerificationCodeHash) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		panic(err) // crypto/rand.Reader never fails
	}
	code := fmt.Sprintf("%06d", n.Int64())
	return code, HashVerificationCode(code)
}

func HashVerificationCode(code string) VerificationCodeHash {
	return sha256.Sum256([]byte(code))
}

// ErrVerificationCodeNotFound is returned when a user has no code.
var ErrVerificationCodeNotFound = errors.New("verification code not found")

type VerificationCodeRepository interface {
	// SaveVerificationCode stores c, replacing the user's previous code.
	SaveVerificationCode(ctx context.Context, c *VerificationCode) error
	// FindVerificationCode returns ErrVerificationCodeNotFound when the user
	// has no code. It may return an expired code; callers check ExpiresAt.
	FindVerificationCode(ctx context.Context, userID uuid.UUID) (*VerificationCode, error)
	// ConsumeVerificationAttempt atomically increments the code's Attempts
	// and returns the updated code, so concurrent guesses cannot get past the
	// limit. It returns ErrVerificationCodeNotFound when there is no code.
	ConsumeVerificationAttempt(ctx context.Context, userID uuid.UUID) (*VerificationCode, error)
	// DeleteVerificationCode is idempotent.
	DeleteVerificationCode(ctx context.Context, userID uuid.UUID) error
}

// Mailer sends the emails the application needs.
type Mailer interface {
	SendVerificationCode(ctx context.Context, email, code string) error
}
