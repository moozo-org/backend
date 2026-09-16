package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecoverHidesPanicFromClient(t *testing.T) {
	const secret = "secret=mongodb://user:pass@host/db"

	logger, logs := observed()
	h := Recover(logger, true, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(secret)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/hello", nil))

	body, _ := io.ReadAll(rec.Result().Body)
	if strings.Contains(string(body), "mongodb") {
		t.Fatalf("panic detail leaked to client: %s", body)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}

	entry := logs.FilterMessage("panic recovered")
	if entry.Len() != 1 {
		t.Fatal("panic was not logged")
	}
	fields := entry.All()[0].ContextMap()
	if fields["panic"] != secret {
		t.Errorf("panic field = %v, want %q", fields["panic"], secret)
	}
	if !strings.Contains(fields["stack"].(string), "goroutine") {
		t.Error("stack missing from log")
	}
}

func TestRecoverPassesThroughErrAbortHandler(t *testing.T) {
	logger, _ := observed()
	h := Recover(logger, true, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	defer func() {
		if rec := recover(); rec != http.ErrAbortHandler {
			t.Errorf("recovered %v, want ErrAbortHandler to propagate", rec)
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

// A panic after the response started must not append an error object.
func TestRecoverAbortsAfterPartialWrite(t *testing.T) {
	logger, logs := observed()
	h := Recover(logger, false, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"hi"}`))
		panic("late boom")
	}))

	rec := httptest.NewRecorder()
	func() {
		defer func() {
			if r := recover(); r != http.ErrAbortHandler {
				t.Errorf("recovered %v, want ErrAbortHandler", r)
			}
		}()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/hello", nil))
	}()

	body, _ := io.ReadAll(rec.Result().Body)
	if got := string(body); got != `{"message":"hi"}` {
		t.Errorf("body = %s, want the partial body untouched", got)
	}
	if f := logs.FilterMessage("panic recovered").All()[0].ContextMap(); f["response_started"] != true {
		t.Error("response_started not recorded")
	}
}
