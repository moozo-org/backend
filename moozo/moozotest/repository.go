// Package moozotest provides in-memory fakes of the moozo repositories, for
// tests of the layers above them.
package moozotest

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"moozo/moozo"
)

// Repository is an in-memory moozo.UserRepository, moozo.SessionRepository
// and moozo.VerificationCodeRepository.
// The zero value is ready to use. It mirrors the MongoDB repository's
// behaviour that callers rely on: case-insensitive unique emails.
type Repository struct {
	// Err, when set, is returned by every method, to simulate an outage.
	Err error

	mu       sync.Mutex
	users    []*moozo.User
	sessions map[moozo.SessionTokenHash]*moozo.Session
	codes    map[uuid.UUID]*moozo.VerificationCode
}

var (
	_ moozo.UserRepository             = (*Repository)(nil)
	_ moozo.SessionRepository          = (*Repository)(nil)
	_ moozo.VerificationCodeRepository = (*Repository)(nil)
)

// Users returns the registered users, in registration order.
func (r *Repository) Users() []*moozo.User {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*moozo.User(nil), r.users...)
}

// Sessions returns the number of stored sessions.
func (r *Repository) Sessions() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.sessions)
}

func (r *Repository) Register(_ context.Context, u *moozo.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	if r.findUser(u.Email) != nil {
		return moozo.ErrEmailTaken
	}
	r.users = append(r.users, u)
	return nil
}

func (r *Repository) FindUserByEmail(_ context.Context, email string) (*moozo.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return nil, r.Err
	}
	if u := r.findUser(email); u != nil {
		return u, nil
	}
	return nil, moozo.ErrUserNotFound
}

func (r *Repository) ActivateUser(_ context.Context, id uuid.UUID, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	for _, u := range r.users {
		if u.ID == id {
			u.Status = moozo.StatusActive
			u.UpdatedAt = at
			return nil
		}
	}
	return moozo.ErrUserNotFound
}

func (r *Repository) findUser(email string) *moozo.User {
	for _, u := range r.users {
		if strings.EqualFold(u.Email, email) {
			return u
		}
	}
	return nil
}

func (r *Repository) CreateSession(_ context.Context, s *moozo.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	if r.sessions == nil {
		r.sessions = make(map[moozo.SessionTokenHash]*moozo.Session)
	}
	r.sessions[s.TokenHash] = s
	return nil
}

func (r *Repository) FindSession(_ context.Context, hash moozo.SessionTokenHash) (*moozo.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return nil, r.Err
	}
	if s, ok := r.sessions[hash]; ok {
		return s, nil
	}
	return nil, moozo.ErrSessionNotFound
}

func (r *Repository) DeleteSession(_ context.Context, hash moozo.SessionTokenHash) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	delete(r.sessions, hash)
	return nil
}

// VerificationCode returns a copy of the user's stored code, or nil.
func (r *Repository) VerificationCode(userID uuid.UUID) *moozo.VerificationCode {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.codes[userID]; ok {
		cp := *c
		return &cp
	}
	return nil
}

func (r *Repository) SaveVerificationCode(_ context.Context, c *moozo.VerificationCode) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	if r.codes == nil {
		r.codes = make(map[uuid.UUID]*moozo.VerificationCode)
	}
	cp := *c
	r.codes[c.UserID] = &cp
	return nil
}

func (r *Repository) FindVerificationCode(_ context.Context, userID uuid.UUID) (*moozo.VerificationCode, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return nil, r.Err
	}
	if c, ok := r.codes[userID]; ok {
		cp := *c
		return &cp, nil
	}
	return nil, moozo.ErrVerificationCodeNotFound
}

func (r *Repository) ConsumeVerificationAttempt(_ context.Context, userID uuid.UUID) (*moozo.VerificationCode, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return nil, r.Err
	}
	c, ok := r.codes[userID]
	if !ok {
		return nil, moozo.ErrVerificationCodeNotFound
	}
	c.Attempts++
	cp := *c
	return &cp, nil
}

func (r *Repository) DeleteVerificationCode(_ context.Context, userID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	delete(r.codes, userID)
	return nil
}
