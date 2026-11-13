package command

import (
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"moozo/moozo"
	"moozo/moozo/moozotest"
)

const newPassword = "battery staple"

// newJaneWithResetCode registers jane with status and emails her a reset
// code, which it returns.
func newJaneWithResetCode(t *testing.T, repo *moozotest.Repository, status moozo.Status) (*moozo.User, string) {
	t.Helper()
	jane := newJaneWithStatus(t, repo, status)
	mailer := &moozotest.Mailer{}
	if _, err := NewForgotPasswordHandler(repo, repo, mailer).Execute(t.Context(), ForgotPassword{Email: jane.Email}); err != nil {
		t.Fatal(err)
	}
	return jane, mailer.SentResets()[0].Code
}

func TestResetPassword(t *testing.T) {
	cases := map[string]struct {
		status moozo.Status // jane's; active when empty
		// before runs after jane got her code; it may change the state.
		before   func(t *testing.T, repo *moozotest.Repository, jane *moozo.User)
		email    string                   // jane's when empty
		code     func(sent string) string // the code sent when nil
		password string                   // newPassword when empty
		repoErr  error
		wantErr  error
		wantCode bool // whether jane still has a code afterwards
	}{
		"right code":          {},
		"email is normalized": {email: "  JANE@example.com "},
		"pending account":     {status: moozo.StatusPending},
		"wrong code":          {code: wrongCode, wantErr: ErrInvalidResetCode, wantCode: true},
		"unknown email":       {email: "eve@example.com", wantErr: ErrInvalidResetCode, wantCode: true},
		"expired code": {
			before:  func(*testing.T, *moozotest.Repository, *moozo.User) { time.Sleep(PasswordResetCodeLifetime) },
			wantErr: ErrInvalidResetCode,
		},
		"no code": {
			before: func(t *testing.T, repo *moozotest.Repository, jane *moozo.User) {
				if _, err := repo.DeletePasswordResetCode(t.Context(), jane.ID); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: ErrInvalidResetCode,
		},
		// Checked first, so the code is not spent on it.
		"password over 72 bytes": {password: strings.Repeat("é", 40), wantErr: ErrPasswordTooLong, wantCode: true},
		"repository failure":     {repoErr: errBoom, wantErr: errBoom, wantCode: true},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				repo := &moozotest.Repository{}
				status := tc.status
				if status == "" {
					status = moozo.StatusActive
				}
				jane, sent := newJaneWithResetCode(t, repo, status)
				oldHash := jane.PasswordHash
				// Jane is logged in somewhere; Bob must stay logged in.
				_, janes := moozo.NewSessionToken()
				_, bobs := moozo.NewSessionToken()
				for _, s := range []*moozo.Session{{TokenHash: janes, UserID: jane.ID}, {TokenHash: bobs, UserID: uuid.Must(uuid.NewV7())}} {
					if err := repo.CreateSession(t.Context(), s); err != nil {
						t.Fatal(err)
					}
				}
				if tc.before != nil {
					tc.before(t, repo, jane)
				}

				cmd := ResetPassword{Email: jane.Email, Code: sent, NewPassword: newPassword}
				if tc.email != "" {
					cmd.Email = tc.email
				}
				if tc.code != nil {
					cmd.Code = tc.code(sent)
				}
				if tc.password != "" {
					cmd.NewPassword = tc.password
				}
				repo.Err = tc.repoErr

				_, err := NewResetPasswordHandler(repo, repo, repo).Execute(t.Context(), cmd)
				repo.Err = nil
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Execute() error = %v, want %v", err, tc.wantErr)
				}
				if got := repo.PasswordResetCode(jane.ID) != nil; got != tc.wantCode {
					t.Errorf("code kept = %v, want %v", got, tc.wantCode)
				}

				if tc.wantErr != nil {
					if jane.PasswordHash != oldHash || jane.Status != status {
						t.Error("account changed despite the error")
					}
					if repo.Sessions() != 2 {
						t.Errorf("sessions = %d, want both kept", repo.Sessions())
					}
					return
				}
				if bcrypt.CompareHashAndPassword([]byte(jane.PasswordHash), []byte(newPassword)) != nil {
					t.Error("new password does not match the stored hash")
				}
				if jane.Status != moozo.StatusActive {
					t.Errorf("Status = %q, want active", jane.Status)
				}
				if _, err := repo.FindSession(t.Context(), janes); err == nil {
					t.Error("jane's session survived the reset")
				}
				if _, err := repo.FindSession(t.Context(), bobs); err != nil {
					t.Error("another user's session was revoked")
				}
			})
		})
	}
}

func TestResetPasswordCodeIsSingleUse(t *testing.T) {
	repo := &moozotest.Repository{}
	jane, sent := newJaneWithResetCode(t, repo, moozo.StatusActive)
	reset := NewResetPasswordHandler(repo, repo, repo)

	if _, err := reset.Execute(t.Context(), ResetPassword{Email: jane.Email, Code: sent, NewPassword: newPassword}); err != nil {
		t.Fatalf("first use: error = %v", err)
	}
	_, err := reset.Execute(t.Context(), ResetPassword{Email: jane.Email, Code: sent, NewPassword: "another password"})
	if !errors.Is(err, ErrInvalidResetCode) {
		t.Fatalf("second use: error = %v, want %v", err, ErrInvalidResetCode)
	}
}

func TestResetPasswordAttemptLimit(t *testing.T) {
	cases := map[string]struct {
		wrong   int // wrong codes tried before the right one
		wantErr error
	}{
		"right code on the last attempt": {wrong: MaxVerificationAttempts - 1},
		"right code after the limit":     {wrong: MaxVerificationAttempts, wantErr: ErrInvalidResetCode},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &moozotest.Repository{}
			jane, sent := newJaneWithResetCode(t, repo, moozo.StatusActive)
			reset := NewResetPasswordHandler(repo, repo, repo)

			for range tc.wrong {
				_, err := reset.Execute(t.Context(), ResetPassword{Email: jane.Email, Code: wrongCode(sent), NewPassword: newPassword})
				if !errors.Is(err, ErrInvalidResetCode) {
					t.Fatalf("wrong code: error = %v", err)
				}
			}
			_, err := reset.Execute(t.Context(), ResetPassword{Email: jane.Email, Code: sent, NewPassword: newPassword})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("right code: error = %v, want %v", err, tc.wantErr)
			}
			if repo.PasswordResetCode(jane.ID) != nil {
				t.Error("code kept after it was used up")
			}
		})
	}
}
