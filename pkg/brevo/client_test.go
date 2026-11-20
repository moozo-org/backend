package brevo

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestClient returns a Client whose requests go to handler.
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewClient("test-key", WithBaseURL(srv.URL))
}

func TestSendEmail(t *testing.T) {
	want := Email{
		Sender:      Contact{Email: "noreply@example.com", Name: "Example"},
		To:          []Contact{{Email: "jane@example.com"}},
		Subject:     "Hello",
		TextContent: "Hi Jane",
	}

	var got Email
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/smtp/email" {
			t.Errorf("request = %s %s, want POST /smtp/email", r.Method, r.URL.Path)
		}
		if r.Header.Get("api-key") != "test-key" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("headers = %v", r.Header)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"messageId":"<1@smtp-relay.mailin.fr>"}`)
	})

	if err := c.SendEmail(t.Context(), want); err != nil {
		t.Fatalf("SendEmail() error = %v", err)
	}
	if got.Sender != want.Sender || len(got.To) != 1 || got.To[0] != want.To[0] ||
		got.Subject != want.Subject || got.TextContent != want.TextContent {
		t.Errorf("sent %+v, want %+v", got, want)
	}
}

func TestSendEmailAPIError(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
		want   APIError
	}{
		"with error body": {
			status: http.StatusBadRequest,
			body:   `{"code":"invalid_parameter","message":"sender is not valid"}`,
			want:   APIError{StatusCode: http.StatusBadRequest, Code: "invalid_parameter", Message: "sender is not valid"},
		},
		"without error body": {
			status: http.StatusBadGateway,
			body:   `<html>bad gateway</html>`,
			want:   APIError{StatusCode: http.StatusBadGateway},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})

			err := c.SendEmail(t.Context(), Email{})
			var apiErr *APIError
			if !errors.As(err, &apiErr) || *apiErr != tc.want {
				t.Fatalf("SendEmail() error = %#v, want %#v", err, tc.want)
			}
		})
	}
}
