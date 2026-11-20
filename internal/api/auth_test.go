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
	return newTestServerWithMailer(t, repo, &moozotest.Mailer{})
}

func newTestServerWithMailer(t *testing.T, repo *moozotest.Repository, mailer *moozotest.Mailer) *generated.Server {
	t.Helper()
	logger, _ := observed()
	h := NewHandler(logger, false, app.New(repo, repo, repo, mailer))
	srv, err := generated.NewServer(h, h, generated.WithErrorHandler(ErrorHandler(logger, false)))
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	return srv
}

// withUser returns a repository holding one active planner with email and
// password.
func withUser(t *testing.T, email, password string) *moozotest.Repository {
	t.Helper()
	return withUserStatus(t, email, password, moozo.StatusActive)
}

func withUserStatus(t *testing.T, email, password string, status moozo.Status) *moozotest.Repository {
	t.Helper()
	repo := &moozotest.Repository{}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	u := &moozo.User{ID: uuid.Must(uuid.NewV7()), Email: email, PasswordHash: string(hash), Role: moozo.RolePlanner, Status: status}
	if err := repo.Register(t.Context(), u); err != nil {
		t.Fatal(err)
	}
	return repo
}

func newRegisterHandler(repo *moozotest.Repository) *Handler {
	logger, _ := observed()
	return NewHandler(logger, false, app.New(repo, repo, repo, &moozotest.Mailer{}))
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
	if got.ID != stored.ID || got.Email != stored.Email || string(got.Role) != string(stored.Role) ||
		got.Status != generated.UserStatusPending {
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

// The account exists, so the response is the same 201 as a sent email.
func TestRegisterMailFailure(t *testing.T) {
	repo := &moozotest.Repository{}
	logger, logs := observed()
	h := NewHandler(logger, false, app.New(repo, repo, repo, &moozotest.Mailer{Err: errors.New("boom")}))

	res, err := h.Register(t.Context(), &generated.RegisterRequest{
		Email:    "jane@example.com",
		Password: "password123",
		Role:     generated.RegisterRolePlanner,
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if _, ok := res.(*generated.User); !ok {
		t.Fatalf("response = %T, want *generated.User", res)
	}
	if logs.FilterMessage("registered user without verification email").Len() != 1 {
		t.Error("mail failure not logged")
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

// The spec's enums are a second copy of the domain's roles and statuses; the
// command would still reject a drifted role, but the published API would
// advertise it.
func TestSpecEnumsMatchDomain(t *testing.T) {
	cases := map[string]struct {
		spec   []string
		domain []string
	}{
		"RegisterRole": {spec: enumValues(generated.RegisterRole("").AllValues()), domain: enumValues(moozo.SelfServiceRoles)},
		"Role":         {spec: enumValues(generated.Role("").AllValues()), domain: enumValues(moozo.Roles)},
		"UserStatus":   {spec: enumValues(generated.UserStatus("").AllValues()), domain: enumValues(moozo.Statuses)},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			slices.Sort(tc.spec)
			slices.Sort(tc.domain)
			if !slices.Equal(tc.spec, tc.domain) {
				t.Errorf("spec %s = %v, domain = %v", name, tc.spec, tc.domain)
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
		status moozo.Status // jane's; active when empty
		body   string
		want   int
	}{
		"valid credentials": {body: janeLogin, want: http.StatusOK},
		"email in another case": {
			body: `{"email":"JANE@example.com","password":"correct horse"}`,
			want: http.StatusOK,
		},
		"wrong password": {body: `{"email":"jane@example.com","password":"wrong horse"}`, want: http.StatusUnauthorized},
		"email not verified": {
			status: moozo.StatusPending,
			body:   janeLogin,
			want:   http.StatusForbidden,
		},
		"unknown email":    {body: `{"email":"eve@example.com","password":"correct horse"}`, want: http.StatusUnauthorized},
		"missing password": {body: `{"email":"jane@example.com"}`, want: http.StatusBadRequest},
		"invalid email":    {body: `{"email":"jane","password":"correct horse"}`, want: http.StatusBadRequest},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			status := tc.status
			if status == "" {
				status = moozo.StatusActive
			}
			srv := newTestServer(t, withUserStatus(t, "jane@example.com", "correct horse", status))

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

const janeRegister = `{"email":"jane@example.com","password":"correct horse","role":"planner"}`

// Sign-up to login, through the API.
func TestVerifyEmailFlowHTTP(t *testing.T) {
	repo := &moozotest.Repository{}
	mailer := &moozotest.Mailer{}
	srv := newTestServerWithMailer(t, repo, mailer)

	if rec := serve(t, srv, "/auth/register", janeRegister, ""); rec.Code != http.StatusCreated {
		t.Fatalf("register: status = %d; body = %s", rec.Code, rec.Body)
	}
	if rec := serve(t, srv, "/auth/login", janeLogin, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("login before verifying: status = %d, want 403", rec.Code)
	}

	sent := mailer.Sent()
	if len(sent) != 1 {
		t.Fatalf("sent %d codes, want 1", len(sent))
	}
	verify := fmt.Sprintf(`{"email":"jane@example.com","code":%q}`, sent[0].Code)
	if rec := serve(t, srv, "/auth/verify", verify, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("verify: status = %d; body = %s", rec.Code, rec.Body)
	}
	login(t, srv)
}

func TestVerifyEmailHTTP(t *testing.T) {
	cases := map[string]struct {
		body string
		want int
	}{
		"wrong code":    {body: `{"email":"jane@example.com","code":"123456"}`, want: http.StatusBadRequest},
		"unknown email": {body: `{"email":"eve@example.com","code":"123456"}`, want: http.StatusBadRequest},
		"short code":    {body: `{"email":"jane@example.com","code":"12345"}`, want: http.StatusBadRequest},
		"letters":       {body: `{"email":"jane@example.com","code":"12345a"}`, want: http.StatusBadRequest},
		"missing code":  {body: `{"email":"jane@example.com"}`, want: http.StatusBadRequest},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			// No code was sent, so no code can be right.
			srv := newTestServer(t, withUserStatus(t, "jane@example.com", "correct horse", moozo.StatusPending))

			if rec := serve(t, srv, "/auth/verify", tc.body, ""); rec.Code != tc.want {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tc.want, rec.Body)
			}
		})
	}
}

func TestResendVerificationHTTP(t *testing.T) {
	cases := map[string]struct {
		body     string
		repoErr  error
		want     int
		wantSent int
	}{
		"pending account": {body: `{"email":"jane@example.com"}`, want: http.StatusAccepted, wantSent: 1},
		"unknown email":   {body: `{"email":"eve@example.com"}`, want: http.StatusAccepted},
		"invalid email":   {body: `{"email":"jane"}`, want: http.StatusBadRequest},
		"database outage": {body: `{"email":"jane@example.com"}`, repoErr: errors.New("boom"), want: http.StatusInternalServerError},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			repo := withUserStatus(t, "jane@example.com", "correct horse", moozo.StatusPending)
			repo.Err = tc.repoErr
			mailer := &moozotest.Mailer{}
			srv := newTestServerWithMailer(t, repo, mailer)

			if rec := serve(t, srv, "/auth/resend-verification", tc.body, ""); rec.Code != tc.want {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tc.want, rec.Body)
			}
			if got := len(mailer.Sent()); got != tc.wantSent {
				t.Errorf("sent %d codes, want %d", got, tc.wantSent)
			}
		})
	}
}
