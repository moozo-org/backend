package command

import (
	"errors"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"moozo/moozo"
	"moozo/moozo/moozotest"
)

const janePassword = "correct horse"

// newJane registers jane@example.com with janePassword. MinCost keeps the
// test fast; Login reads the cost from the hash.
func newJane(t *testing.T, repo *moozotest.Repository) *moozo.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(janePassword), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	u := &moozo.User{ID: uuid.Must(uuid.NewV7()), Email: "jane@example.com", PasswordHash: string(hash), Role: moozo.RolePlanner}
	if err := repo.Register(t.Context(), u); err != nil {
		t.Fatal(err)
	}
	return u
}

func TestLogin(t *testing.T) {
	cases := map[string]struct {
		cmd     Login
		repoErr error
		wantErr error
	}{
		"valid credentials":   {cmd: Login{Email: "jane@example.com", Password: janePassword}},
		"email is normalized": {cmd: Login{Email: "  JANE@example.com ", Password: janePassword}},
		"wrong password": {
			cmd:     Login{Email: "jane@example.com", Password: "wrong horse"},
			wantErr: ErrInvalidCredentials,
		},
		// Same error as a wrong password, so the response does not reveal
		// which emails have accounts.
		"unknown email": {
			cmd:     Login{Email: "eve@example.com", Password: janePassword},
			wantErr: ErrInvalidCredentials,
		},
		"password over 72 bytes": {
			cmd:     Login{Email: "jane@example.com", Password: strings.Repeat("é", 40)},
			wantErr: ErrInvalidCredentials,
		},
		"repository failure": {
			cmd:     Login{Email: "jane@example.com", Password: janePassword},
			repoErr: errBoom,
			wantErr: errBoom,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				repo := &moozotest.Repository{}
				jane := newJane(t, repo)
				repo.Err = tc.repoErr

				res, err := NewLoginHandler(repo, repo).Execute(t.Context(), tc.cmd)
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Execute() error = %v, want %v", err, tc.wantErr)
				}
				if tc.wantErr != nil {
					if repo.Sessions() != 0 {
						t.Error("session created for a failed login")
					}
					return
				}

				assertLoggedIn(t, res, jane, repo)
			})
		})
	}
}

func assertLoggedIn(t *testing.T, res *LoginResult, user *moozo.User, repo *moozotest.Repository) {
	t.Helper()

	if res.User != user {
		t.Errorf("User = %+v, want %+v", res.User, user)
	}
	s := res.Session
	if s.UserID != user.ID || s.Role != user.Role {
		t.Errorf("session user/role = %s/%s, want %s/%s", s.UserID, s.Role, user.ID, user.Role)
	}
	if s.TokenHash != moozo.HashSessionToken(res.Token) {
		t.Error("session TokenHash is not the hash of the returned token")
	}
	if !s.CreatedAt.Equal(synctestEpoch) || !s.ExpiresAt.Equal(synctestEpoch.Add(SessionLifetime)) {
		t.Errorf("CreatedAt/ExpiresAt = %v/%v, want %v/%v",
			s.CreatedAt, s.ExpiresAt, synctestEpoch, synctestEpoch.Add(SessionLifetime))
	}
	if stored, err := repo.FindSession(t.Context(), s.TokenHash); err != nil || stored != s {
		t.Errorf("stored session = %+v, %v; want the returned session", stored, err)
	}
}

func TestLoginTokensAreUnique(t *testing.T) {
	repo := &moozotest.Repository{}
	newJane(t, repo)
	login := NewLoginHandler(repo, repo)

	first, err := login.Execute(t.Context(), Login{Email: "jane@example.com", Password: janePassword})
	if err != nil {
		t.Fatal(err)
	}
	second, err := login.Execute(t.Context(), Login{Email: "jane@example.com", Password: janePassword})
	if err != nil {
		t.Fatal(err)
	}
	if first.Token == second.Token {
		t.Error("two logins returned the same token")
	}
	if repo.Sessions() != 2 {
		t.Errorf("stored %d sessions, want 2", repo.Sessions())
	}
}
