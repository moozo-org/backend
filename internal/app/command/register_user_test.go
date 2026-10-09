package command

import (
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"golang.org/x/crypto/bcrypt"

	"moozo/moozo"
	"moozo/moozo/moozotest"
)

var errBoom = errors.New("boom")

// synctestEpoch is where time.Now starts inside a synctest bubble.
var synctestEpoch = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

func TestRegisterUser(t *testing.T) {
	cases := map[string]struct {
		cmd      RegisterUser
		existing []*moozo.User // registered before cmd runs
		repoErr  error
		wantErr  error
	}{
		"planner":  {cmd: RegisterUser{Email: "jane@example.com", Password: "correct horse", Role: moozo.RolePlanner}},
		"provider": {cmd: RegisterUser{Email: "jane@example.com", Password: "correct horse", Role: moozo.RoleProvider}},
		"email is normalized": {
			cmd: RegisterUser{Email: "  Jane@Example.com ", Password: "correct horse", Role: moozo.RolePlanner},
		},
		"admin role": {
			cmd:     RegisterUser{Email: "eve@example.com", Password: "password123", Role: moozo.RoleAdmin},
			wantErr: ErrRoleNotSelfService,
		},
		"unknown role": {
			cmd:     RegisterUser{Email: "eve@example.com", Password: "password123", Role: "root"},
			wantErr: ErrRoleNotSelfService,
		},
		"empty role": {
			cmd:     RegisterUser{Email: "eve@example.com", Password: "password123"},
			wantErr: ErrRoleNotSelfService,
		},
		// 40 characters, 80 bytes: passes the spec's maxLength, fails bcrypt.
		"password over 72 bytes": {
			cmd:     RegisterUser{Email: "jane@example.com", Password: strings.Repeat("é", 40), Role: moozo.RolePlanner},
			wantErr: ErrPasswordTooLong,
		},
		"email taken": {
			cmd:      RegisterUser{Email: "Jane@example.com", Password: "password123", Role: moozo.RolePlanner},
			existing: []*moozo.User{{Email: "jane@example.com"}},
			wantErr:  moozo.ErrEmailTaken,
		},
		"repository failure": {
			cmd:     RegisterUser{Email: "jane@example.com", Password: "password123", Role: moozo.RolePlanner},
			repoErr: errBoom,
			wantErr: errBoom,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				repo := &moozotest.Repository{}
				for _, u := range tc.existing {
					if err := repo.Register(t.Context(), u); err != nil {
						t.Fatal(err)
					}
				}
				repo.Err = tc.repoErr

				u, err := NewRegisterUserHandler(repo).Execute(t.Context(), tc.cmd)
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Execute() error = %v, want %v", err, tc.wantErr)
				}
				if tc.wantErr != nil {
					if len(repo.Users()) != len(tc.existing) {
						t.Error("rejected user was registered")
					}
					return
				}

				assertRegistered(t, tc.cmd, u, repo)
			})
		})
	}
}

// assertRegistered checks the user Execute built from cmd and handed to repo.
func assertRegistered(t *testing.T, cmd RegisterUser, u *moozo.User, repo *moozotest.Repository) {
	t.Helper()

	if want := strings.ToLower(strings.TrimSpace(cmd.Email)); u.Email != want {
		t.Errorf("Email = %q, want %q", u.Email, want)
	}
	if u.Role != cmd.Role {
		t.Errorf("Role = %q, want %q", u.Role, cmd.Role)
	}
	if u.ID.Version() != 7 {
		t.Errorf("ID %s is UUID v%d, want v7", u.ID, u.ID.Version())
	}
	if !u.CreatedAt.Equal(synctestEpoch) || !u.UpdatedAt.Equal(synctestEpoch) {
		t.Errorf("CreatedAt/UpdatedAt = %v/%v, want %v", u.CreatedAt, u.UpdatedAt, synctestEpoch)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(cmd.Password)); err != nil {
		t.Errorf("hash does not match password: %v", err)
	}
	if users := repo.Users(); len(users) != 1 || users[0] != u {
		t.Errorf("repository got %v, want the returned user", users)
	}
}
