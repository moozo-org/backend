package command

import (
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"moozo/moozo"
	"moozo/moozo/moozotest"
)

// newPendingJane registers a pending jane and emails her a code, which it
// returns.
func newPendingJane(t *testing.T, repo *moozotest.Repository) (*moozo.User, string) {
	t.Helper()
	jane := newJaneWithStatus(t, repo, moozo.StatusPending)
	mailer := &moozotest.Mailer{}
	if err := (sendVerificationCode{codes: repo, mailer: mailer}).send(t.Context(), jane); err != nil {
		t.Fatal(err)
	}
	return jane, mailer.Sent()[0].Code
}

// wrongCode returns a valid-looking code other than code.
func wrongCode(code string) string {
	if code == "000000" {
		return "000001"
	}
	return "000000"
}

func TestVerifyEmail(t *testing.T) {
	cases := map[string]struct {
		// before runs after jane got her code; it may change the state.
		before   func(t *testing.T, repo *moozotest.Repository, jane *moozo.User)
		email    string                   // jane's when empty
		code     func(sent string) string // the code sent when nil
		repoErr  error
		wantErr  error
		wantCode bool // whether jane still has a code afterwards
	}{
		"right code":          {},
		"email is normalized": {email: "  JANE@example.com "},
		"wrong code": {
			code:     wrongCode,
			wantErr:  ErrInvalidVerificationCode,
			wantCode: true,
		},
		"unknown email": {
			email:    "eve@example.com",
			wantErr:  ErrInvalidVerificationCode,
			wantCode: true,
		},
		"expired code": {
			before:  func(*testing.T, *moozotest.Repository, *moozo.User) { time.Sleep(VerificationCodeLifetime) },
			wantErr: ErrInvalidVerificationCode,
		},
		"no code": {
			before: func(t *testing.T, repo *moozotest.Repository, jane *moozo.User) {
				if err := repo.DeleteVerificationCode(t.Context(), jane.ID); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: ErrInvalidVerificationCode,
		},
		// A retried request must not fail once the first one went through.
		"already active": {
			before: func(t *testing.T, repo *moozotest.Repository, jane *moozo.User) {
				if err := repo.ActivateUser(t.Context(), jane.ID, time.Now()); err != nil {
					t.Fatal(err)
				}
			},
			code:     wrongCode,
			wantCode: true,
		},
		"repository failure": {
			repoErr:  errBoom,
			wantErr:  errBoom,
			wantCode: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				repo := &moozotest.Repository{}
				jane, sent := newPendingJane(t, repo)
				if tc.before != nil {
					tc.before(t, repo, jane)
				}
				cmd := VerifyEmail{Email: jane.Email, Code: sent}
				if tc.email != "" {
					cmd.Email = tc.email
				}
				if tc.code != nil {
					cmd.Code = tc.code(sent)
				}
				repo.Err = tc.repoErr

				_, err := NewVerifyEmailHandler(repo, repo).Execute(t.Context(), cmd)
				repo.Err = nil
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Execute() error = %v, want %v", err, tc.wantErr)
				}

				wantStatus := moozo.StatusPending
				if tc.wantErr == nil {
					wantStatus = moozo.StatusActive
				}
				if jane.Status != wantStatus {
					t.Errorf("Status = %q, want %q", jane.Status, wantStatus)
				}
				if got := repo.VerificationCode(jane.ID) != nil; got != tc.wantCode {
					t.Errorf("code kept = %v, want %v", got, tc.wantCode)
				}
			})
		})
	}
}

func TestVerifyEmailAttemptLimit(t *testing.T) {
	cases := map[string]struct {
		wrong   int // wrong codes tried before the right one
		wantErr error
	}{
		"right code on the last attempt": {wrong: MaxVerificationAttempts - 1},
		"right code after the limit":     {wrong: MaxVerificationAttempts, wantErr: ErrInvalidVerificationCode},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &moozotest.Repository{}
			jane, sent := newPendingJane(t, repo)
			verify := NewVerifyEmailHandler(repo, repo)

			for range tc.wrong {
				_, err := verify.Execute(t.Context(), VerifyEmail{Email: jane.Email, Code: wrongCode(sent)})
				if !errors.Is(err, ErrInvalidVerificationCode) {
					t.Fatalf("wrong code: error = %v", err)
				}
			}
			_, err := verify.Execute(t.Context(), VerifyEmail{Email: jane.Email, Code: sent})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("right code: error = %v, want %v", err, tc.wantErr)
			}
			if repo.VerificationCode(jane.ID) != nil {
				t.Error("code kept after it was used up")
			}
		})
	}
}
