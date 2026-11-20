package command

import (
	"cmp"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"moozo/moozo"
	"moozo/moozo/moozotest"
)

const changedPassword = "battery staple"

func TestChangePassword(t *testing.T) {
	cases := map[string]struct {
		current string // the current password the caller sends
		next    string // the new password; changedPassword when empty
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
			s := logInJaneTwiceAndBob(t, repo, jane)
			repo.Err = tc.repoErr

			userID := jane.ID
			if tc.unknown {
				userID = uuid.Must(uuid.NewV7())
			}

			_, err := NewChangePasswordHandler(repo, repo).Execute(t.Context(), ChangePassword{
				UserID:          userID,
				Session:         s.current,
				CurrentPassword: tc.current,
				NewPassword:     cmp.Or(tc.next, changedPassword),
			})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Execute() error = %v, want %v", err, tc.wantErr)
			}
			repo.Err = nil

			if tc.wantErr != nil {
				checkChangeRejected(t, repo, jane, oldHash)
			} else {
				checkChangeApplied(t, repo, jane, s)
			}
		})
	}
}

// changeSessions are the token hashes of the sessions a change password test
// starts with.
type changeSessions struct {
	other   moozo.SessionTokenHash // jane's, not the one asking
	current moozo.SessionTokenHash // jane's, the one asking for the change
	bobs    moozo.SessionTokenHash // another user's
}

// logInJaneTwiceAndBob logs jane in twice and another user once. Bob's
// session must survive whatever happens to jane's.
func logInJaneTwiceAndBob(t *testing.T, repo *moozotest.Repository, jane *moozo.User) changeSessions {
	t.Helper()
	var s changeSessions
	_, s.other = moozo.NewSessionToken()
	_, s.current = moozo.NewSessionToken()
	_, s.bobs = moozo.NewSessionToken()
	for _, session := range []*moozo.Session{
		{TokenHash: s.other, UserID: jane.ID},
		{TokenHash: s.current, UserID: jane.ID},
		{TokenHash: s.bobs, UserID: uuid.Must(uuid.NewV7())},
	} {
		if err := repo.CreateSession(t.Context(), session); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

// checkChangeRejected checks that a failed change kept jane's password and
// every session.
func checkChangeRejected(t *testing.T, repo *moozotest.Repository, jane *moozo.User, oldHash string) {
	t.Helper()
	if jane.PasswordHash != oldHash {
		t.Error("password changed despite the error")
	}
	if repo.Sessions() != 3 {
		t.Errorf("sessions = %d, want all 3 kept", repo.Sessions())
	}
}

// checkChangeApplied checks that jane has the new password and that only her
// other session was revoked.
func checkChangeApplied(t *testing.T, repo *moozotest.Repository, jane *moozo.User, s changeSessions) {
	t.Helper()
	if bcrypt.CompareHashAndPassword([]byte(jane.PasswordHash), []byte(changedPassword)) != nil {
		t.Error("new password does not match the stored hash")
	}
	if jane.UpdatedAt.IsZero() {
		t.Error("UpdatedAt not set")
	}
	for hash, want := range map[moozo.SessionTokenHash]bool{s.other: false, s.current: true, s.bobs: true} {
		_, err := repo.FindSession(t.Context(), hash)
		if got := err == nil; got != want {
			t.Errorf("session kept = %t, want %t", got, want)
		}
	}
}
