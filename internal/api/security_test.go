package api

import (
	"errors"
	"net/http"
	"testing"

	"github.com/ogen-go/ogen/ogenerrors"
	"go.uber.org/zap/zapcore"

	"moozo/internal/app"
	"moozo/internal/app/query"
	"moozo/moozo/moozotest"
)

// Security failures skip LoggingMiddleware, so NewError must log them, and
// only client mistakes may become 401s.
func TestNewErrorSecurityFailures(t *testing.T) {
	security := func(err error) error { return &ogenerrors.SecurityError{Security: "BearerAuth", Err: err} }

	cases := map[string]struct {
		err       error
		wantCode  int
		wantLevel zapcore.Level
		wantLog   bool
	}{
		"invalid token": {err: security(query.ErrUnauthenticated), wantCode: http.StatusUnauthorized, wantLevel: zapcore.WarnLevel, wantLog: true},
		"no token": {
			err:      &ogenerrors.SecurityError{Err: ogenerrors.ErrSecurityRequirementIsNotSatisfied},
			wantCode: http.StatusUnauthorized, wantLevel: zapcore.WarnLevel, wantLog: true,
		},
		"database outage": {err: security(errors.New("boom")), wantCode: http.StatusInternalServerError, wantLevel: zapcore.ErrorLevel, wantLog: true},
		// LoggingMiddleware already logged it.
		"handler error": {err: errors.New("boom"), wantCode: http.StatusInternalServerError},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			logger, logs := observed()
			h := NewHandler(logger, true, app.New(&moozotest.Repository{}, &moozotest.Repository{}))

			got := h.NewError(t.Context(), tc.err)
			if got.StatusCode != tc.wantCode {
				t.Errorf("status = %d, want %d", got.StatusCode, tc.wantCode)
			}

			entries := logs.FilterMessage("authentication failed").All()
			if !tc.wantLog {
				if len(entries) != 0 {
					t.Errorf("logged %d entries, want none", len(entries))
				}
				return
			}
			if len(entries) != 1 || entries[0].Level != tc.wantLevel {
				t.Errorf("entries = %+v, want one at %s", entries, tc.wantLevel)
			}
		})
	}
}
