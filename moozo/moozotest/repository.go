// Package moozotest provides in-memory fakes of the moozo repositories, for
// tests of the layers above them.
package moozotest

import (
	"context"
	"strings"
	"sync"

	"moozo/moozo"
)

// Repository is an in-memory moozo.UserRepository and moozo.SessionRepository.
// The zero value is ready to use. It mirrors the MongoDB repository's
// behaviour that callers rely on: case-insensitive unique emails.
type Repository struct {
	// Err, when set, is returned by every method, to simulate an outage.
	Err error

	mu       sync.Mutex
	users    []*moozo.User
	sessions map[moozo.SessionTokenHash]*moozo.Session
}

var (
	_ moozo.UserRepository    = (*Repository)(nil)
	_ moozo.SessionRepository = (*Repository)(nil)
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
