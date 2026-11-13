package command

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"moozo/moozo"
	"moozo/moozo/moozotest"
)

func TestChangePassword(t *testing.T) {
	const newPassword = "battery staple"

	cases := map[string]struct {
		current string // the current password the caller sends
		next    string // the new password; newPassword when empty
		unknown bool   // the session's user does not exist
		repoErr error
		wantErr error
	}{
		"valid change":           {current: janePassword},
		"wrong current password": {current: "wrong horse", wantErr: ErrWrongPassword},
		"current over 72 bytes":  {current: strings.Repeat("é", 40), wantErr: ErrWrongPassword},
		"same password":          {current: janePassword, next: janePassword, wantErr: ErrSamePassword},
		"new over 72 bytes":      {current: janePassword, next: strings.Repeat("é", 40), wantErr: ErrPasswordTooLong},
		"unknown user":           {current: janePassword, unknown: true, wantErr: moozo.ErrUserNotFound},
		"repository failure":     {current: janePassword, repoErr: errBoom, wantErr: errBoom},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &moozotest.Repository{}
			jane := newJane(t, repo)
			oldHash := jane.PasswordHash

			// Jane is logged in twice; the second session asks for the change.
			// Bob's session must survive whatever happens to Jane's.
			_, other := moozo.NewSessionToken()
			_, current := moozo.NewSessionToken()
			_, bobs := moozo.NewSessionToken()
			for _, s := range []*moozo.Session{
				{TokenHash: other, UserID: jane.ID},
				{TokenHash: current, UserID: jane.ID},
				{TokenHash: bobs, UserID: uuid.Must(uuid.NewV7())},
			} {
				if err := repo.CreateSession(t.Context(), s); err != nil {
					t.Fatal(err)
				}
			}
			repo.Err = tc.repoErr

			next := tc.next
			if next == "" {
				next = newPassword
			}
			userID := jane.ID
			if tc.unknown {
				userID = uuid.Must(uuid.NewV7())
			}

			_, err := NewChangePasswordHandler(repo, repo).Execute(t.Context(), ChangePassword{
				UserID: userID, Session: current, CurrentPassword: tc.current, NewPassword: next,
			})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Execute() error = %v, want %v", err, tc.wantErr)
			}
			repo.Err = nil

			if tc.wantErr != nil {
				if jane.PasswordHash != oldHash {
					t.Error("password changed despite the error")
				}
				if repo.Sessions() != 3 {
					t.Errorf("sessions = %d, want all 3 kept", repo.Sessions())
				}
				return
			}

			if bcrypt.CompareHashAndPassword([]byte(jane.PasswordHash), []byte(newPassword)) != nil {
				t.Error("new password does not match the stored hash")
			}
			if jane.UpdatedAt.IsZero() {
				t.Error("UpdatedAt not set")
			}
			for hash, want := range map[moozo.SessionTokenHash]bool{other: false, current: true, bobs: true} {
				_, err := repo.FindSession(t.Context(), hash)
				if got := err == nil; got != want {
					t.Errorf("session kept = %t, want %t", got, want)
				}
			}
		})
	}
}
