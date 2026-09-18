package api

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func observed() (*zap.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(zap.DebugLevel)
	return zap.New(core), logs
}

func newTestHandler(production bool) *Handler {
	logger, _ := observed()
	return NewHandler(logger, production)
}

func TestErrorHandlerHidesDetailFromClient(t *testing.T) {
	logger, logs := observed()
	rec := httptest.NewRecorder()

	ErrorHandler(logger, true)(t.Context(), rec,
		httptest.NewRequest(http.MethodGet, "/hello", nil), io.ErrUnexpectedEOF)

	body, _ := io.ReadAll(rec.Result().Body)
	if strings.Contains(string(body), "unexpected EOF") {
		t.Fatalf("internal error leaked to client: %s", body)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	if logs.FilterMessage("request failed").Len() != 1 {
		t.Fatal("error was not logged")
	}
	if !strings.Contains(logs.All()[0].ContextMap()["error"].(string), "unexpected EOF") {
		t.Fatal("error detail missing from log")
	}
}

func TestWithTraceInertWithoutProvider(t *testing.T) {
	logger, logs := observed()
	withTrace(t.Context(), logger).Info("x")

	if _, ok := logs.All()[0].ContextMap()["trace_id"]; ok {
		t.Fatal("trace_id emitted with no TracerProvider registered")
	}
}

func TestErrorBodyDebugOnlyOutsideProduction(t *testing.T) {
	const detail = "mongo: dial tcp 10.0.0.5:27017: refused"
	err := errors.New(detail)

	prod := errorBody(500, err, true)
	if prod.Debug.Set {
		t.Errorf("debug present in production: %q", prod.Debug.Value)
	}
	if prod.ErrorMessage != "Internal Server Error" {
		t.Errorf("ErrorMessage = %q", prod.ErrorMessage)
	}

	dev := errorBody(500, err, false)
	if !dev.Debug.Set || dev.Debug.Value != detail {
		t.Errorf("debug = %+v, want %q", dev.Debug, detail)
	}
}

func TestNewErrorRespectsProduction(t *testing.T) {
	err := errors.New("boom: secret=mongodb://u:p@h/db")

	got := newTestHandler(true).NewError(t.Context(), err)
	if got.StatusCode != 500 {
		t.Errorf("StatusCode = %d, want 500", got.StatusCode)
	}
	payload, _ := got.Response.MarshalJSON()
	if strings.Contains(string(payload), "secret") {
		t.Fatalf("detail leaked in production: %s", payload)
	}

	got = newTestHandler(false).NewError(t.Context(), err)
	payload, _ = got.Response.MarshalJSON()
	if !strings.Contains(string(payload), "secret") {
		t.Fatalf("detail missing outside production: %s", payload)
	}
}
