package moozo

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/google/uuid"
)

// SessionTokenHash identifies a session. Only the hash of a token is stored,
// so a leaked sessions collection does not hand out working tokens.
type SessionTokenHash [sha256.Size]byte

// Session is a logged-in user. Its token is never stored; see NewSessionToken.
type Session struct {
	TokenHash SessionTokenHash
	UserID    uuid.UUID
	// Role is the user's role at login, so authorization needs no user
	// lookup. Whatever changes a user's role must delete their sessions, or
	// the old role lasts until the session expires.
	Role      Role
	CreatedAt time.Time
	ExpiresAt time.Time
}

// NewSessionToken returns a random bearer token and the hash to store. 256
// bits of entropy make a fast hash safe here, unlike passwords.
func NewSessionToken() (string, SessionTokenHash) {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // crypto/rand.Read never returns an error
	token := base64.RawURLEncoding.EncodeToString(b)
	return token, HashSessionToken(token)
}

func HashSessionToken(token string) SessionTokenHash {
	return sha256.Sum256([]byte(token))
}

// ErrSessionNotFound is returned when no session matches a token hash.
var ErrSessionNotFound = errors.New("session not found")

type SessionRepository interface {
	CreateSession(ctx context.Context, s *Session) error
	// FindSession returns ErrSessionNotFound when no session has the hash. It
	// may return an expired session; callers check ExpiresAt.
	FindSession(ctx context.Context, hash SessionTokenHash) (*Session, error)
	// DeleteSession is idempotent.
	DeleteSession(ctx context.Context, hash SessionTokenHash) error
}
