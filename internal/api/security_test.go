package api

import (
	"errors"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ogen-go/ogen/ogenerrors"
	"go.uber.org/zap/zapcore"
	"go.yaml.in/yaml/v3"

	"moozo/internal/api/openapi/generated"
	"moozo/internal/app"
	"moozo/internal/app/query"
	"moozo/moozo"
	"moozo/moozo/moozotest"
)

const authFailed = "authentication failed"

// Security failures skip LoggingMiddleware, so NewError must log them, and
// only client mistakes may become 401s.
func TestNewErrorSecurityFailures(t *testing.T) {
	security := func(err error) error { return &ogenerrors.SecurityError{Security: "BearerAuth", Err: err} }

	cases := map[string]struct {
		err       error
		wantCode  int
		wantLevel zapcore.Level
		wantLog   string // the message NewError logs; none when empty
	}{
		"invalid token": {err: security(query.ErrUnauthenticated), wantCode: http.StatusUnauthorized, wantLevel: zapcore.WarnLevel, wantLog: authFailed},
		"no token": {
			err:      &ogenerrors.SecurityError{Err: ogenerrors.ErrSecurityRequirementIsNotSatisfied},
			wantCode: http.StatusUnauthorized, wantLevel: zapcore.WarnLevel, wantLog: authFailed,
		},
		"wrong role":      {err: security(ErrForbidden), wantCode: http.StatusForbidden, wantLevel: zapcore.WarnLevel, wantLog: "access denied"},
		"database outage": {err: security(errors.New("boom")), wantCode: http.StatusInternalServerError, wantLevel: zapcore.ErrorLevel, wantLog: authFailed},
		// LoggingMiddleware already logged it.
		"handler error": {err: errors.New("boom"), wantCode: http.StatusInternalServerError},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			logger, logs := observed()
			h := NewHandler(logger, true, app.New(&moozotest.Repository{}, &moozotest.Repository{}, &moozotest.Repository{}, &moozotest.Mailer{}))

			got := h.NewError(t.Context(), tc.err)
			if got.StatusCode != tc.wantCode {
				t.Errorf("status = %d, want %d", got.StatusCode, tc.wantCode)
			}

			if tc.wantLog == "" {
				if n := logs.Len(); n != 0 {
					t.Errorf("logged %d entries, want none", n)
				}
				return
			}
			entries := logs.All()
			if len(entries) != 1 || entries[0].Message != tc.wantLog || entries[0].Level != tc.wantLevel {
				t.Errorf("entries = %+v, want one at %s", entries, tc.wantLevel)
			}
		})
	}
}

func TestHandleBearerAuthChecksRoles(t *testing.T) {
	cases := map[string]struct {
		role    moozo.Role
		allowed []string // the operation's roles in the spec
		wantErr error
	}{
		"no roles listed":    {role: moozo.RoleProvider},
		"role listed":        {role: moozo.RolePlanner, allowed: []string{"planner"}},
		"one of several":     {role: moozo.RoleAdmin, allowed: []string{"planner", "admin"}},
		"role not listed":    {role: moozo.RoleProvider, allowed: []string{"planner"}, wantErr: ErrForbidden},
		"admin not implicit": {role: moozo.RoleAdmin, allowed: []string{"provider"}, wantErr: ErrForbidden},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &moozotest.Repository{}
			token, hash := moozo.NewSessionToken()
			err := repo.CreateSession(t.Context(), &moozo.Session{
				TokenHash: hash, UserID: uuid.Must(uuid.NewV7()), Role: tc.role, ExpiresAt: time.Now().Add(time.Hour),
			})
			if err != nil {
				t.Fatal(err)
			}
			logger, _ := observed()
			h := NewHandler(logger, false, app.New(repo, repo, repo, &moozotest.Mailer{}))

			ctx, err := h.HandleBearerAuth(t.Context(), "", generated.BearerAuth{Token: token, Roles: tc.allowed})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if _, ok := sessionFrom(ctx); ok != (tc.wantErr == nil) {
				t.Errorf("session in context = %t, want %t", ok, tc.wantErr == nil)
			}
		})
	}
}

// A role misspelled in the spec would lock everyone out of the operation.
func TestSpecSecurityRolesAreDomainRoles(t *testing.T) {
	var spec struct {
		Paths map[string]map[string]struct {
			Security []map[string][]string `yaml:"security"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(specFile, &spec); err != nil {
		t.Fatal(err)
	}

	for path, ops := range spec.Paths {
		for method, op := range ops {
			for _, requirement := range op.Security {
				for scheme, roles := range requirement {
					for _, r := range roles {
						if !slices.Contains(moozo.Roles, moozo.Role(r)) {
							t.Errorf("%s %s: %s role %q is not in moozo.Roles", method, path, scheme, r)
						}
					}
				}
			}
		}
	}
}
