package mongodb

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"moozo/moozo"
)

func newSession() *moozo.Session {
	_, hash := moozo.NewSessionToken()
	now := time.Now().UTC().Truncate(time.Millisecond)
	return &moozo.Session{
		TokenHash: hash,
		UserID:    uuid.Must(uuid.NewV7()),
		Role:      moozo.RoleProvider,
		CreatedAt: now,
		ExpiresAt: now.Add(time.Hour),
	}
}

func TestSessions(t *testing.T) {
	cases := map[string]struct {
		// act runs against a repository already holding stored; it returns
		// the session FindSession should then find, or nil for none.
		act func(t *testing.T, repo *Repository, stored *moozo.Session) *moozo.Session
	}{
		"created session is found": {
			act: func(_ *testing.T, _ *Repository, stored *moozo.Session) *moozo.Session { return stored },
		},
		"deleted session is gone": {
			act: func(t *testing.T, repo *Repository, stored *moozo.Session) *moozo.Session {
				if err := repo.DeleteSession(t.Context(), stored.TokenHash); err != nil {
					t.Fatalf("DeleteSession() error = %v", err)
				}
				return nil
			},
		},
		"deleting twice is fine": {
			act: func(t *testing.T, repo *Repository, stored *moozo.Session) *moozo.Session {
				for range 2 {
					if err := repo.DeleteSession(t.Context(), stored.TokenHash); err != nil {
						t.Fatalf("DeleteSession() error = %v", err)
					}
				}
				return nil
			},
		},
		"deleting another session keeps this one": {
			act: func(t *testing.T, repo *Repository, stored *moozo.Session) *moozo.Session {
				if err := repo.DeleteSession(t.Context(), newSession().TokenHash); err != nil {
					t.Fatalf("DeleteSession() error = %v", err)
				}
				return stored
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			repo := newRepository(t)
			stored := newSession()
			if err := repo.CreateSession(t.Context(), stored); err != nil {
				t.Fatalf("CreateSession() error = %v", err)
			}

			want := tc.act(t, repo, stored)

			got, err := repo.FindSession(t.Context(), stored.TokenHash)
			if want == nil {
				if !errors.Is(err, moozo.ErrSessionNotFound) {
					t.Fatalf("FindSession() = %+v, %v; want ErrSessionNotFound", got, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("FindSession() error = %v", err)
			}
			if *got != *want {
				t.Errorf("found %+v, want %+v", *got, *want)
			}
		})
	}
}
