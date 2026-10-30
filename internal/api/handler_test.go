package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServeSpecReturnsEmbeddedSpec(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestHandler(false).ServeSpec(rec, httptest.NewRequest(http.MethodGet, "/docs/openapi.yaml", nil))

	if got := rec.Header().Get("Content-Type"); got != "application/yaml" {
		t.Errorf("Content-Type = %q, want application/yaml", got)
	}
	body, _ := io.ReadAll(rec.Result().Body)
	if len(body) == 0 {
		t.Fatal("empty spec")
	}
}
