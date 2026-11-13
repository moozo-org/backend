package command

import (
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"moozo/moozo"
	"moozo/moozo/moozotest"
)

func TestForgotPassword(t *testing.T) {
	cases := map[string]struct {
		status  moozo.Status // jane's; active when empty
		before  func(t *testing.T, repo *moozotest.Repository, jane *moozo.User)
		email   string // jane's when empty
		repoErr error
		wantErr error
		wantNew bool // whether a new code is sent
	}{
		"active account":      {wantNew: true},
		"pending account":     {status: moozo.StatusPending, wantNew: true},
		"email is normalized": {email: " JANE@example.com", wantNew: true},
		"after the cooldown":  {before: askResetThenWait(ResendCooldown), wantNew: true},
		"within the cooldown": {before: askResetThenWait(ResendCooldown - time.Second)},
		// Answers like a sent code, so the endpoint does not reveal which
		// emails have accounts.
		"unknown email":      {email: "eve@example.com"},
		"repository failure": {repoErr: errBoom, wantErr: errBoom},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				repo := &moozotest.Repository{}
				status := tc.status
				if status == "" {
					status = moozo.StatusActive
				}
				jane := newJaneWithStatus(t, repo, status)
				if tc.before != nil {
					tc.before(t, repo, jane)
				}
				email := jane.Email
				if tc.email != "" {
					email = tc.email
				}
				repo.Err = tc.repoErr
				mailer := &moozotest.Mailer{}

				_, err := NewForgotPasswordHandler(repo, repo, mailer).Execute(t.Context(), ForgotPassword{Email: email})
				repo.Err = nil
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Execute() error = %v, want %v", err, tc.wantErr)
				}
				sent := mailer.SentResets()
				if !tc.wantNew {
					if len(sent) != 0 {
						t.Errorf("sent %+v, want nothing", sent)
					}
					return
				}

				if len(sent) != 1 || sent[0].Email != jane.Email {
					t.Fatalf("sent %+v, want one code to %s", sent, jane.Email)
				}
				c := repo.PasswordResetCode(jane.ID)
				if c == nil {
					t.Fatal("no code stored")
				}
				if c.CodeHash != moozo.HashVerificationCode(sent[0].Code) {
					t.Error("stored hash does not match the emailed code")
				}
				if got := c.ExpiresAt.Sub(c.CreatedAt); got != PasswordResetCodeLifetime {
					t.Errorf("lifetime = %v, want %v", got, PasswordResetCodeLifetime)
				}
				if len(mailer.Sent()) != 0 || repo.VerificationCode(jane.ID) != nil {
					t.Error("a verification code was sent or stored")
				}
			})
		})
	}
}

// A reset code must not replace the code a pending user verifies with.
func TestForgotPasswordKeepsVerificationCode(t *testing.T) {
	repo := &moozotest.Repository{}
	jane, _ := newPendingJane(t, repo)
	before := repo.VerificationCode(jane.ID)

	if _, err := NewForgotPasswordHandler(repo, repo, &moozotest.Mailer{}).Execute(t.Context(), ForgotPassword{Email: jane.Email}); err != nil {
		t.Fatal(err)
	}
	if after := repo.VerificationCode(jane.ID); after == nil || *after != *before {
		t.Errorf("verification code = %+v, want %+v", after, before)
	}
}

// askResetThenWait gets jane a reset code, then lets d pass.
func askResetThenWait(d time.Duration) func(*testing.T, *moozotest.Repository, *moozo.User) {
	return func(t *testing.T, repo *moozotest.Repository, jane *moozo.User) {
		t.Helper()
		if _, err := NewForgotPasswordHandler(repo, repo, &moozotest.Mailer{}).Execute(t.Context(), ForgotPassword{Email: jane.Email}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(d)
	}
}
