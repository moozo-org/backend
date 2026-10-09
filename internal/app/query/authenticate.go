package query

import (
	"context"
	"errors"
	"fmt"
	"time"

	"moozo/moozo"
)

// ErrUnauthenticated is returned for an unknown, revoked or expired token.
var ErrUnauthenticated = errors.New("invalid or expired session")

type Authenticate struct {
	Token string
}

type AuthenticateHandler struct {
	sessions moozo.SessionRepository
}

func NewAuthenticateHandler(sessions moozo.SessionRepository) AuthenticateHandler {
	return AuthenticateHandler{sessions: sessions}
}

func (h AuthenticateHandler) Get(ctx context.Context, q Authenticate) (*moozo.Session, error) {
	s, err := h.sessions.FindSession(ctx, moozo.HashSessionToken(q.Token))
	if errors.Is(err, moozo.ErrSessionNotFound) {
		return nil, ErrUnauthenticated
	}
	if err != nil {
		return nil, fmt.Errorf("find session: %w", err)
	}
	// MongoDB's TTL monitor deletes expired sessions only about once a minute.
	if !time.Now().Before(s.ExpiresAt) {
		return nil, ErrUnauthenticated
	}
	return s, nil
}
