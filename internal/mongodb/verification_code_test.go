package mongodb

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"moozo/moozo"
)

func newVerificationCode(userID uuid.UUID) *moozo.VerificationCode {
	_, hash := moozo.NewVerificationCode()
	now := time.Now().UTC().Truncate(time.Millisecond)
	return &moozo.VerificationCode{
		UserID:    userID,
		CodeHash:  hash,
		CreatedAt: now,
		ExpiresAt: now.Add(15 * time.Minute),
	}
}

func TestVerificationCodes(t *testing.T) {
	cases := map[string]struct {
		// act runs against a repository already holding stored; it returns
		// the code FindVerificationCode should then find, or nil for none.
		act func(t *testing.T, repo *Repository, stored *moozo.VerificationCode) *moozo.VerificationCode
	}{
		"saved code is found": {
			act: func(_ *testing.T, _ *Repository, stored *moozo.VerificationCode) *moozo.VerificationCode {
				return stored
			},
		},
		"saving again replaces the code": {
			act: func(t *testing.T, repo *Repository, stored *moozo.VerificationCode) *moozo.VerificationCode {
				replacement := newVerificationCode(stored.UserID)
				if err := repo.SaveVerificationCode(t.Context(), replacement); err != nil {
					t.Fatalf("SaveVerificationCode() error = %v", err)
				}
				return replacement
			},
		},
		"consuming attempts counts them": {act: consumeTwice},
		"deleted code is gone": {
			act: func(t *testing.T, repo *Repository, stored *moozo.VerificationCode) *moozo.VerificationCode {
				for range 2 { // idempotent
					if err := repo.DeleteVerificationCode(t.Context(), stored.UserID); err != nil {
						t.Fatalf("DeleteVerificationCode() error = %v", err)
					}
				}
				return nil
			},
		},
		"deleting another user's code keeps this one": {
			act: func(t *testing.T, repo *Repository, stored *moozo.VerificationCode) *moozo.VerificationCode {
				if err := repo.DeleteVerificationCode(t.Context(), uuid.Must(uuid.NewV7())); err != nil {
					t.Fatalf("DeleteVerificationCode() error = %v", err)
				}
				return stored
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			repo := newRepository(t)
			stored := newVerificationCode(uuid.Must(uuid.NewV7()))
			if err := repo.SaveVerificationCode(t.Context(), stored); err != nil {
				t.Fatalf("SaveVerificationCode() error = %v", err)
			}

			want := tc.act(t, repo, stored)

			got, err := repo.FindVerificationCode(t.Context(), stored.UserID)
			if want == nil {
				if !errors.Is(err, moozo.ErrVerificationCodeNotFound) {
					t.Fatalf("FindVerificationCode() = %+v, %v; want ErrVerificationCodeNotFound", got, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("FindVerificationCode() error = %v", err)
			}
			if *got != *want {
				t.Errorf("found %+v, want %+v", *got, *want)
			}
		})
	}
}

// consumeTwice spends two attempts on stored and checks the returned code.
func consumeTwice(t *testing.T, repo *Repository, stored *moozo.VerificationCode) *moozo.VerificationCode {
	t.Helper()
	var got *moozo.VerificationCode
	for range 2 {
		var err error
		if got, err = repo.ConsumeVerificationAttempt(t.Context(), stored.UserID); err != nil {
			t.Fatalf("ConsumeVerificationAttempt() error = %v", err)
		}
	}
	want := *stored
	want.Attempts = 2
	if *got != want {
		t.Errorf("ConsumeVerificationAttempt() = %+v, want %+v", *got, want)
	}
	return &want
}

func TestConsumeVerificationAttemptWithoutCode(t *testing.T) {
	repo := newRepository(t)
	_, err := repo.ConsumeVerificationAttempt(t.Context(), uuid.Must(uuid.NewV7()))
	if !errors.Is(err, moozo.ErrVerificationCodeNotFound) {
		t.Errorf("ConsumeVerificationAttempt() error = %v, want ErrVerificationCodeNotFound", err)
	}
}

// Reset codes live in their own collection: saving one keeps the user's
// verification code, and deleting reports whether there was one.
func TestPasswordResetCodes(t *testing.T) {
	repo := newRepository(t)
	userID := uuid.Must(uuid.NewV7())
	verification, reset := newVerificationCode(userID), newVerificationCode(userID)
	if err := repo.SaveVerificationCode(t.Context(), verification); err != nil {
		t.Fatal(err)
	}
	if err := repo.SavePasswordResetCode(t.Context(), reset); err != nil {
		t.Fatalf("SavePasswordResetCode() error = %v", err)
	}

	if got, err := repo.FindVerificationCode(t.Context(), userID); err != nil || *got != *verification {
		t.Errorf("verification code = %+v, %v; want %+v", got, err, verification)
	}
	got, err := repo.ConsumePasswordResetAttempt(t.Context(), userID)
	if err != nil {
		t.Fatalf("ConsumePasswordResetAttempt() error = %v", err)
	}
	want := *reset
	want.Attempts = 1
	if *got != want {
		t.Errorf("consumed %+v, want %+v", *got, want)
	}

	for i, wantDeleted := range []bool{true, false} {
		deleted, err := repo.DeletePasswordResetCode(t.Context(), userID)
		if err != nil || deleted != wantDeleted {
			t.Errorf("delete #%d = %v, %v; want %v", i+1, deleted, err, wantDeleted)
		}
	}
	if _, err := repo.FindPasswordResetCode(t.Context(), userID); !errors.Is(err, moozo.ErrVerificationCodeNotFound) {
		t.Errorf("after delete: error = %v, want ErrVerificationCodeNotFound", err)
	}
}
