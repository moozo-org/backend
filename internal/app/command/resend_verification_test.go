package command

import (
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"moozo/moozo"
	"moozo/moozo/moozotest"
)

func TestResendVerification(t *testing.T) {
	cases := map[string]struct {
		status  moozo.Status // jane's
		before  func(t *testing.T, repo *moozotest.Repository, jane *moozo.User)
		email   string // jane's when empty
		repoErr error
		wantErr error
		wantNew bool // whether a new code is sent
	}{
		"pending, no code yet": {status: moozo.StatusPending, wantNew: true},
		"email is normalized":  {status: moozo.StatusPending, email: " JANE@example.com", wantNew: true},
		"pending, after the cooldown": {
			status:  moozo.StatusPending,
			before:  sendCodeThenWait(ResendCooldown),
			wantNew: true,
		},
		"pending, within the cooldown": {
			status: moozo.StatusPending,
			before: sendCodeThenWait(ResendCooldown - time.Second),
		},
		// The silent cases answer like a sent code, so the endpoint does not
		// reveal which emails have pending accounts.
		"active":        {status: moozo.StatusActive},
		"unknown email": {status: moozo.StatusPending, email: "eve@example.com"},
		"repository failure": {
			status:  moozo.StatusPending,
			repoErr: errBoom,
			wantErr: errBoom,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				repo := &moozotest.Repository{}
				jane := newJaneWithStatus(t, repo, tc.status)
				if tc.before != nil {
					tc.before(t, repo, jane)
				}
				email := jane.Email
				if tc.email != "" {
					email = tc.email
				}
				repo.Err = tc.repoErr
				mailer := &moozotest.Mailer{}

				_, err := NewResendVerificationHandler(repo, repo, mailer).Execute(t.Context(), ResendVerification{Email: email})
				repo.Err = nil
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Execute() error = %v, want %v", err, tc.wantErr)
				}
				if !tc.wantNew {
					if sent := mailer.Sent(); len(sent) != 0 {
						t.Errorf("sent %+v, want nothing", sent)
					}
					return
				}
				assertCodeSent(t, jane, repo, mailer)
			})
		})
	}
}

// sendCodeThenWait sends jane a code, then lets d pass.
func sendCodeThenWait(d time.Duration) func(*testing.T, *moozotest.Repository, *moozo.User) {
	return func(t *testing.T, repo *moozotest.Repository, jane *moozo.User) {
		t.Helper()
		if err := (sendVerificationCode{codes: repo, mailer: &moozotest.Mailer{}}).send(t.Context(), jane); err != nil {
			t.Fatal(err)
		}
		time.Sleep(d)
	}
}

// A resent code replaces the previous one.
func TestResendVerificationReplacesCode(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		repo := &moozotest.Repository{}
		jane, first := newPendingJane(t, repo)
		time.Sleep(ResendCooldown)

		mailer := &moozotest.Mailer{}
		if _, err := NewResendVerificationHandler(repo, repo, mailer).Execute(t.Context(), ResendVerification{Email: jane.Email}); err != nil {
			t.Fatal(err)
		}
		second := mailer.Sent()[0].Code

		verify := NewVerifyEmailHandler(repo, repo)
		if first != second {
			if _, err := verify.Execute(t.Context(), VerifyEmail{Email: jane.Email, Code: first}); !errors.Is(err, ErrInvalidVerificationCode) {
				t.Fatalf("old code: error = %v, want %v", err, ErrInvalidVerificationCode)
			}
		}
		if _, err := verify.Execute(t.Context(), VerifyEmail{Email: jane.Email, Code: second}); err != nil {
			t.Fatalf("new code: error = %v", err)
		}
	})
}
