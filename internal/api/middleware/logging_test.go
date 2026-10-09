package middleware

import (
	"errors"
	"strings"
	"testing"

	"github.com/ogen-go/ogen/middleware"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"moozo/internal/api/openapi/generated"
)

func observed() (*zap.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(zap.DebugLevel)
	return zap.New(core), logs
}

func TestLoggingMiddlewareLogsSuccess(t *testing.T) {
	logger, logs := observed()

	req := middleware.Request{Context: t.Context(), OperationName: "Register", OperationID: "register"}
	_, err := LoggingMiddleware(logger)(req, func(middleware.Request) (middleware.Response, error) {
		return middleware.Response{Type: &generated.User{}}, nil
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}

	entry := logs.FilterMessage("request served")
	if entry.Len() != 1 {
		t.Fatal("success was not logged")
	}
	fields := entry.All()[0].ContextMap()
	if fields["operation"] != "Register" || fields["operation_id"] != "register" {
		t.Errorf("fields = %v", fields)
	}
	if _, ok := fields["took_ms"]; !ok {
		t.Error("took_ms missing")
	}
}

// Failures must keep the operation context that NewError cannot supply.
func TestLoggingMiddlewareLogsFailureWithContext(t *testing.T) {
	logger, logs := observed()
	want := errors.New("boom")

	req := middleware.Request{Context: t.Context(), OperationName: "Register", OperationID: "register"}
	_, err := LoggingMiddleware(logger)(req, func(middleware.Request) (middleware.Response, error) {
		return middleware.Response{}, want
	})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}

	entry := logs.FilterMessage("request failed")
	if entry.Len() != 1 {
		t.Fatal("failure was not logged")
	}
	got := entry.All()[0]
	if got.Level != zapcore.ErrorLevel {
		t.Errorf("level = %v, want error", got.Level)
	}
	fields := got.ContextMap()
	if fields["operation"] != "Register" {
		t.Errorf("operation missing: %v", fields)
	}
	if fields["status_code"] != int64(500) {
		t.Errorf("status_code = %v, want 500", fields["status_code"])
	}
	if _, ok := fields["took_ms"]; !ok {
		t.Error("took_ms missing")
	}
}

func TestLogAt(t *testing.T) {
	cases := map[string]struct {
		code int
		want zapcore.Level
	}{
		"bad request":            {code: 400, want: zapcore.WarnLevel},
		"unsupported media type": {code: 415, want: zapcore.WarnLevel},
		"internal server error":  {code: 500, want: zapcore.ErrorLevel},
		"not implemented":        {code: 501, want: zapcore.ErrorLevel},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			core, logs := observer.New(zap.DebugLevel)
			LogAt(zap.New(core, zap.AddCaller()), tc.code, "m")

			e := logs.All()[0]
			if e.Level != tc.want {
				t.Errorf("logged at %v, want %v", e.Level, tc.want)
			}
			// The caller is this test, not LogAt in logging.go.
			if !strings.HasSuffix(e.Caller.File, "logging_test.go") {
				t.Errorf("caller = %s, want this test", e.Caller)
			}
		})
	}
}

func TestWithTraceInertWithoutProvider(t *testing.T) {
	logger, logs := observed()
	WithTrace(t.Context(), logger).Info("x")

	if _, ok := logs.All()[0].ContextMap()["trace_id"]; ok {
		t.Fatal("trace_id emitted with no TracerProvider registered")
	}
}
