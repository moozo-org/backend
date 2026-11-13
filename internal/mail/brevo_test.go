package mail

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"moozo/pkg/brevo"
)

// The HTTP side is tested in pkg/brevo; this checks the emails' content.
func TestBrevoSendCode(t *testing.T) {
	cases := map[string]struct {
		send     func(b *Brevo) error
		subject  string
		lifetime string
	}{
		"verification": {
			send:     func(b *Brevo) error { return b.SendVerificationCode(t.Context(), "jane@example.com", "042917") },
			subject:  "Your Moozo verification code",
			lifetime: "15 minutes",
		},
		"password reset": {
			send:     func(b *Brevo) error { return b.SendPasswordResetCode(t.Context(), "jane@example.com", "042917") },
			subject:  "Reset your Moozo password",
			lifetime: "30 minutes",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var got brevo.Email
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
					t.Errorf("decode body: %v", err)
				}
				w.WriteHeader(http.StatusCreated)
			}))
			t.Cleanup(srv.Close)
			b := NewBrevo(brevo.NewClient("test-key", brevo.WithBaseURL(srv.URL)), "noreply@moozo.test", "Moozo")

			if err := tc.send(b); err != nil {
				t.Fatalf("send error = %v", err)
			}

			if got.Sender != (brevo.Contact{Email: "noreply@moozo.test", Name: "Moozo"}) {
				t.Errorf("sender = %+v", got.Sender)
			}
			if len(got.To) != 1 || got.To[0].Email != "jane@example.com" {
				t.Errorf("to = %+v", got.To)
			}
			if got.Subject != tc.subject {
				t.Errorf("subject = %q, want %q", got.Subject, tc.subject)
			}
			for kind, content := range map[string]string{"text": got.TextContent, "html": got.HTMLContent} {
				if !strings.Contains(content, "042917") || !strings.Contains(content, tc.lifetime) {
					t.Errorf("%s content %q lacks the code or %s", kind, content, tc.lifetime)
				}
			}
		})
	}
}
