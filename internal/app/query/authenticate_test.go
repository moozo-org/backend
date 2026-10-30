package query

import (
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"

	"moozo/moozo"
	"moozo/moozo/moozotest"
)

var errBoom = errors.New("boom")

func TestAuthenticate(t *testing.T) {
	cases := map[string]struct {
		token   func(valid string) string // the token presented, given the stored one
		elapsed time.Duration             // time passed since login
		repoErr error
		wantErr error
	}{
		"valid token":   {token: func(valid string) string { return valid }},
		"unknown token": {token: func(string) string { return "not-a-token" }, wantErr: ErrUnauthenticated},
		"expired":       {token: func(valid string) string { return valid }, elapsed: time.Hour, wantErr: ErrUnauthenticated},
		"last instant":  {token: func(valid string) string { return valid }, elapsed: time.Hour - time.Millisecond},
		// An outage must not look like a bad token: it would turn into a 401
		// instead of a 500.
		"repository failure": {token: func(valid string) string { return valid }, repoErr: errBoom, wantErr: errBoom},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				repo := &moozotest.Repository{}
				token, hash := moozo.NewSessionToken()
				stored := &moozo.Session{
					TokenHash: hash,
					UserID:    uuid.Must(uuid.NewV7()),
					CreatedAt: time.Now(),
					ExpiresAt: time.Now().Add(time.Hour),
				}
				if err := repo.CreateSession(t.Context(), stored); err != nil {
					t.Fatal(err)
				}
				repo.Err = tc.repoErr
				time.Sleep(tc.elapsed) // instant inside the bubble

				s, err := NewAuthenticateHandler(repo).Get(t.Context(), Authenticate{Token: tc.token(token)})
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Get() error = %v, want %v", err, tc.wantErr)
				}
				if tc.wantErr == nil && s != stored {
					t.Errorf("session = %+v, want %+v", s, stored)
				}
			})
		})
	}
}
