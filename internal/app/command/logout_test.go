package command

import (
	"errors"
	"testing"

	"moozo/moozo"
	"moozo/moozo/moozotest"
)

func TestLogout(t *testing.T) {
	_, hash := moozo.NewSessionToken()

	cases := map[string]struct {
		existing bool // whether the session exists before logout
		repoErr  error
		wantErr  error
	}{
		"existing session": {existing: true},
		// Logging out twice, or with a session the TTL already removed, is
		// not an error.
		"session already gone": {},
		"repository failure":   {existing: true, repoErr: errBoom, wantErr: errBoom},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &moozotest.Repository{}
			if tc.existing {
				if err := repo.CreateSession(t.Context(), &moozo.Session{TokenHash: hash}); err != nil {
					t.Fatal(err)
				}
			}
			repo.Err = tc.repoErr

			_, err := NewLogoutHandler(repo).Execute(t.Context(), Logout{TokenHash: hash})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Execute() error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr == nil && repo.Sessions() != 0 {
				t.Error("session still stored after logout")
			}
		})
	}
}
