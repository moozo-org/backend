package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"moozo/internal/api/openapi/generated"
	"moozo/internal/app"
	"moozo/moozo"
	"moozo/moozo/moozotest"
)

// The auth rules are tested in internal/app; these tests cover the mapping
// between HTTP and the commands and queries.

// newTestServer serves the full generated API, so the spec's validation and
// security run too.
func newTestServer(t *testing.T, repo *moozotest.Repository) *generated.Server {
	t.Helper()
	logger, _ := observed()
	h := NewHandler(logger, false, app.New(repo, repo))
	srv, err := generated.NewServer(h, h, generated.WithErrorHandler(ErrorHandler(logger, false)))
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	return srv
}

// withUser returns a repository holding one planner with email and password.
func withUser(t *testing.T, email, password string) *moozotest.Repository {
	t.Helper()
	repo := &moozotest.Repository{}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	u := &moozo.User{ID: uuid.Must(uuid.NewV7()), Email: email, PasswordHash: string(hash), Role: moozo.RolePlanner}
	if err := repo.Register(t.Context(), u); err != nil {
		t.Fatal(err)
	}
	return repo
}

func newRegisterHandler(repo *moozotest.Repository) *Handler {
	logger, _ := observed()
	return NewHandler(logger, false, app.New(repo, repo))
}

func TestRegisterReturnsCreatedUser(t *testing.T) {
	repo := &moozotest.Repository{}
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
	users := repo.Users()
	if len(users) != 1 {
		t.Fatalf("registered %d users, want 1", len(users))
	}
	stored := users[0]
	if got.ID != stored.ID || got.Email != stored.Email || string(got.Role) != string(stored.Role) {
		t.Errorf("response %+v does not match stored user %+v", got, stored)
	}
}

func TestRegisterMapsErrors(t *testing.T) {
	tests := []struct {
		name string
		repo *moozotest.Repository
		req  generated.RegisterRequest
		want any
	}{
		{
			// Bypasses the spec's enum, which normally rejects admin first.
			name: "admin role",
			repo: &moozotest.Repository{},
			req:  generated.RegisterRequest{Email: "eve@example.com", Password: "password123", Role: generated.RegisterRole(moozo.RoleAdmin)},
			want: &generated.RegisterBadRequest{},
		},
		{
			name: "password over 72 bytes",
			repo: &moozotest.Repository{},
			req:  generated.RegisterRequest{Email: "jane@example.com", Password: strings.Repeat("é", 40), Role: generated.RegisterRolePlanner},
			want: &generated.RegisterBadRequest{},
		},
		{
			name: "email taken",
			repo: withUser(t, "jane@example.com", "password123"),
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
	_, err := newRegisterHandler(&moozotest.Repository{Err: errors.New("boom")}).Register(t.Context(), &generated.RegisterRequest{
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
			repo := &moozotest.Repository{}
			srv := newTestServer(t, repo)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			srv.ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tt.want, rec.Body)
			}
			if tt.want != http.StatusCreated {
				if len(repo.Users()) != 0 {
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

func serve(t *testing.T, srv http.Handler, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

const janeLogin = `{"email":"jane@example.com","password":"correct horse"}`

func TestLoginHTTP(t *testing.T) {
	cases := map[string]struct {
		body string
		want int
	}{
		"valid credentials": {body: janeLogin, want: http.StatusOK},
		"email in another case": {
			body: `{"email":"JANE@example.com","password":"correct horse"}`,
			want: http.StatusOK,
		},
		"wrong password":   {body: `{"email":"jane@example.com","password":"wrong horse"}`, want: http.StatusUnauthorized},
		"unknown email":    {body: `{"email":"eve@example.com","password":"correct horse"}`, want: http.StatusUnauthorized},
		"missing password": {body: `{"email":"jane@example.com"}`, want: http.StatusBadRequest},
		"invalid email":    {body: `{"email":"jane","password":"correct horse"}`, want: http.StatusBadRequest},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := newTestServer(t, withUser(t, "jane@example.com", "correct horse"))

			rec := serve(t, srv, "/auth/login", tc.body, "")
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tc.want, rec.Body)
			}
			if tc.want != http.StatusOK {
				return
			}

			var got generated.Session
			if err := got.UnmarshalJSON(rec.Body.Bytes()); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if got.Token == "" || got.User.Email != "jane@example.com" || !got.ExpiresAt.After(got.User.CreatedAt) {
				t.Errorf("session = %+v", got)
			}
		})
	}
}

// login returns a fresh token for jane through the API.
func login(t *testing.T, srv http.Handler) string {
	t.Helper()
	rec := serve(t, srv, "/auth/login", janeLogin, "")
	var s generated.Session
	if rec.Code != http.StatusOK || s.UnmarshalJSON(rec.Body.Bytes()) != nil {
		t.Fatalf("login: status %d, body %s", rec.Code, rec.Body)
	}
	return s.Token
}

func TestLogoutHTTP(t *testing.T) {
	cases := map[string]struct {
		token   func(valid string) string // the token sent, given a logged-in one
		repoErr error                     // set after login
		want    int
	}{
		"valid session":   {token: func(valid string) string { return valid }, want: http.StatusNoContent},
		"no token":        {token: func(string) string { return "" }, want: http.StatusUnauthorized},
		"unknown token":   {token: func(string) string { return "not-a-token" }, want: http.StatusUnauthorized},
		"database outage": {token: func(valid string) string { return valid }, repoErr: errors.New("boom"), want: http.StatusInternalServerError},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			repo := withUser(t, "jane@example.com", "correct horse")
			srv := newTestServer(t, repo)
			valid := login(t, srv)
			repo.Err = tc.repoErr

			rec := serve(t, srv, "/auth/logout", "", tc.token(valid))
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tc.want, rec.Body)
			}
			if tc.want != http.StatusNoContent {
				return
			}

			// The token is revoked: using it again is rejected.
			if rec := serve(t, srv, "/auth/logout", "", valid); rec.Code != http.StatusUnauthorized {
				t.Errorf("reused token: status = %d, want 401", rec.Code)
			}
		})
	}
}
