package httpapi

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hardwareops/control-plane/internal/store/memory"
)

func TestHealth(t *testing.T) {
	deps := Dependencies{Store: memory.New()}
	router := NewRouter(logDiscard(), deps)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func logDiscard() *log.Logger {
	return log.New(&bytes.Buffer{}, "", 0)
}
