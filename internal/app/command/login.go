package command

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"moozo/moozo"
)

// SessionLifetime is how long a login stays valid.
const SessionLifetime = 7 * 24 * time.Hour

// ErrInvalidCredentials covers both an unknown email and a wrong password, so
// callers cannot tell which emails have accounts.
var ErrInvalidCredentials = errors.New("invalid email or password")

type Login struct {
	Email    string
	Password string
}

// LoginResult carries the only copy of the token; it is not stored.
type LoginResult struct {
	Token   string
	Session *moozo.Session
	User    *moozo.User
}

type LoginHandler struct {
	users    moozo.UserRepository
	sessions moozo.SessionRepository
}

func NewLoginHandler(users moozo.UserRepository, sessions moozo.SessionRepository) LoginHandler {
	return LoginHandler{users: users, sessions: sessions}
}

// dummyHash is compared against when the email is unknown, so that path costs
// one bcrypt comparison too and response time does not reveal which emails
// have accounts.
var dummyHash = sync.OnceValue(func() []byte {
	h, err := bcrypt.GenerateFromPassword([]byte("not a real password"), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	return h
})

func (h LoginHandler) Execute(ctx context.Context, cmd Login) (*LoginResult, error) {
	u, err := h.users.FindUserByEmail(ctx, strings.ToLower(strings.TrimSpace(cmd.Email)))
	if errors.Is(err, moozo.ErrUserNotFound) {
		_ = bcrypt.CompareHashAndPassword(dummyHash(), []byte(cmd.Password))
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("find user: %w", err)
	}

	err = bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(cmd.Password))
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) || errors.Is(err, bcrypt.ErrPasswordTooLong) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("compare password: %w", err)
	}

	token, hash := moozo.NewSessionToken()
	// MongoDB stores dates at millisecond precision
	now := time.Now().UTC().Truncate(time.Millisecond)
	s := &moozo.Session{
		TokenHash: hash,
		UserID:    u.ID,
		Role:      u.Role,
		CreatedAt: now,
		ExpiresAt: now.Add(SessionLifetime),
	}
	if err := h.sessions.CreateSession(ctx, s); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return &LoginResult{Token: token, Session: s, User: u}, nil
}
