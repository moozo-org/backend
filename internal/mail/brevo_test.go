package mail

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"moozo/pkg/brevo"
)

// The HTTP side is tested in pkg/brevo; this checks the email's content.
func TestBrevoSendVerificationCode(t *testing.T) {
	var got brevo.Email
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(srv.Close)
	b := NewBrevo(brevo.NewClient("test-key", brevo.WithBaseURL(srv.URL)), "noreply@moozo.test", "Moozo")

	if err := b.SendVerificationCode(t.Context(), "jane@example.com", "042917"); err != nil {
		t.Fatalf("SendVerificationCode() error = %v", err)
	}

	if got.Sender != (brevo.Contact{Email: "noreply@moozo.test", Name: "Moozo"}) {
		t.Errorf("sender = %+v", got.Sender)
	}
	if len(got.To) != 1 || got.To[0].Email != "jane@example.com" {
		t.Errorf("to = %+v", got.To)
	}
	for name, content := range map[string]string{"text": got.TextContent, "html": got.HTMLContent} {
		if !strings.Contains(content, "042917") || !strings.Contains(content, "15 minutes") {
			t.Errorf("%s content %q lacks the code or its lifetime", name, content)
		}
	}
}
