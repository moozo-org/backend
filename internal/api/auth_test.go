package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"moozo/internal/api/openapi/generated"
	"moozo/internal/app"
	"moozo/moozo"
)

// The registration rules are tested in internal/app/command; these tests
// cover the mapping between HTTP and the command.

type fakeUserRepository struct {
	registered []*moozo.User
	err        error
}

func (f *fakeUserRepository) Register(_ context.Context, u *moozo.User) error {
	if f.err != nil {
		return f.err
	}
	f.registered = append(f.registered, u)
	return nil
}

func newRegisterHandler(repo *fakeUserRepository) *Handler {
	logger, _ := observed()
	return NewHandler(logger, false, app.New(repo))
}

func TestRegisterReturnsCreatedUser(t *testing.T) {
	repo := &fakeUserRepository{}
	res, err := newRegisterHandler(repo).Register(t.Context(), &generated.RegisterRequest{
		Email:    "Jane@Example.com",
		Password: "correct horse",
		Role:     generated.RegisterRoleProvider,
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	got, ok := res.(*generated.User)
	if !ok {
		t.Fatalf("response = %T, want *generated.User", res)
	}
	if len(repo.registered) != 1 {
		t.Fatalf("registered %d users, want 1", len(repo.registered))
	}
	stored := repo.registered[0]
	if got.ID != stored.ID || got.Email != stored.Email || string(got.Role) != string(stored.Role) {
		t.Errorf("response %+v does not match stored user %+v", got, stored)
	}
}

func TestRegisterMapsErrors(t *testing.T) {
	tests := []struct {
		name string
		repo *fakeUserRepository
		req  generated.RegisterRequest
		want any
	}{
		{
			// Bypasses the spec's enum, which normally rejects admin first.
			name: "admin role",
			repo: &fakeUserRepository{},
			req:  generated.RegisterRequest{Email: "eve@example.com", Password: "password123", Role: generated.RegisterRole(moozo.RoleAdmin)},
			want: &generated.RegisterBadRequest{},
		},
		{
			name: "password over 72 bytes",
			repo: &fakeUserRepository{},
			req:  generated.RegisterRequest{Email: "jane@example.com", Password: strings.Repeat("é", 40), Role: generated.RegisterRolePlanner},
			want: &generated.RegisterBadRequest{},
		},
		{
			name: "email taken",
			repo: &fakeUserRepository{err: moozo.ErrEmailTaken},
			req:  generated.RegisterRequest{Email: "jane@example.com", Password: "password123", Role: generated.RegisterRolePlanner},
			want: &generated.RegisterConflict{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := newRegisterHandler(tt.repo).Register(t.Context(), &tt.req)
			if err != nil {
				t.Fatalf("Register() error = %v", err)
			}
			if gotType, wantType := fmt.Sprintf("%T", res), fmt.Sprintf("%T", tt.want); gotType != wantType {
				t.Fatalf("response = %s, want %s", gotType, wantType)
			}
		})
	}
}

func TestRegisterRepositoryFailure(t *testing.T) {
	_, err := newRegisterHandler(&fakeUserRepository{err: errors.New("boom")}).Register(t.Context(), &generated.RegisterRequest{
		Email:    "jane@example.com",
		Password: "password123",
		Role:     generated.RegisterRolePlanner,
	})
	if err == nil {
		t.Fatal("Register() error = nil, want repository error")
	}
}

// Through the generated server, so the spec's validation is exercised too.
func TestRegisterHTTP(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"planner", `{"email":"a@example.com","password":"password123","role":"planner"}`, http.StatusCreated},
		{"provider", `{"email":"b@example.com","password":"password123","role":"provider"}`, http.StatusCreated},
		{"admin", `{"email":"c@example.com","password":"password123","role":"admin"}`, http.StatusBadRequest},
		{"unknown role", `{"email":"d@example.com","password":"password123","role":"root"}`, http.StatusBadRequest},
		{"missing role", `{"email":"e@example.com","password":"password123"}`, http.StatusBadRequest},
		{"bad email", `{"email":"nope","password":"password123","role":"planner"}`, http.StatusBadRequest},
		{"short password", `{"email":"f@example.com","password":"short","role":"planner"}`, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeUserRepository{}
			logger, _ := observed()
			srv, err := generated.NewServer(NewHandler(logger, false, app.New(repo)),
				generated.WithErrorHandler(ErrorHandler(logger, false)),
			)
			if err != nil {
				t.Fatalf("NewServer() error = %v", err)
			}

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			srv.ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tt.want, rec.Body)
			}
			if tt.want != http.StatusCreated {
				if len(repo.registered) != 0 {
					t.Error("rejected request was registered")
				}
				return
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if _, leaked := body["password_hash"]; leaked {
				t.Error("response leaks password_hash")
			}
		})
	}
}

// The spec's enums are a second copy of the domain's roles; the command would
// still reject a drifted role, but the published API would advertise it.
func TestSpecRolesMatchDomain(t *testing.T) {
	cases := map[string]struct {
		spec   []string
		domain []moozo.Role
	}{
		"RegisterRole": {spec: enumValues(generated.RegisterRole("").AllValues()), domain: moozo.SelfServiceRoles},
		"Role":         {spec: enumValues(generated.Role("").AllValues()), domain: moozo.Roles},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			domain := enumValues(tc.domain)
			slices.Sort(tc.spec)
			slices.Sort(domain)
			if !slices.Equal(tc.spec, domain) {
				t.Errorf("spec %s = %v, domain = %v", name, tc.spec, domain)
			}
		})
	}
}

func enumValues[E ~string](values []E) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = string(v)
	}
	return out
}
