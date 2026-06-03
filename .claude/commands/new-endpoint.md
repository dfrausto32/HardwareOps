# /new-endpoint

Scaffold a new API endpoint for either the **regional control-plane** or the **global-plane**, following the project's established patterns.

## What to ask if not provided

- **Target**: regional or global-plane?
- **Resource name**: e.g., `widgets`
- **Operations**: which HTTP methods (GET list, GET single, POST, PATCH, DELETE)?
- **Auth level**: viewer / operator / admin (or a service token scope)?

## Regional control-plane pattern

### 1. Handler file: `control-plane/internal/httpapi/handlers/<resource>.go`

```go
package handlers

import (
    "encoding/json"
    "log"
    "net/http"

    "github.com/go-chi/chi/v5"
    "github.com/parcel/control-plane/internal/store"
)

// Use a narrow interface — only methods this handler actually calls.
type widgetStore interface {
    ListWidgets() ([]store.Widget, error)
    CreateWidget(w store.Widget) error
    GetWidget(widgetID string) (store.Widget, bool, error)
    DeleteWidget(widgetID string) error
}

func ListWidgets(logger *log.Logger, st widgetStore, trustProxy bool) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        items, err := st.ListWidgets()
        if err != nil {
            http.Error(w, "storage error", http.StatusInternalServerError)
            return
        }
        if items == nil {
            items = []store.Widget{}
        }
        w.Header().Set("Content-Type", "application/json")
        _ = json.NewEncoder(w).Encode(items)
    }
}
```

### 2. Route in `control-plane/internal/httpapi/router.go`

```go
r.With(viewer).Get("/widgets", handlers.ListWidgets(logger, deps.Store, deps.TrustProxy))
r.With(operator).Post("/widgets", handlers.CreateWidget(logger, deps.Store, deps.TrustProxy))
r.Route("/widgets/{widgetId}", func(r chi.Router) {
    r.With(viewer).Get("/", handlers.GetWidget(logger, deps.Store, deps.TrustProxy))
    r.With(operator).Delete("/", handlers.DeleteWidget(logger, deps.Store, deps.TrustProxy))
})
```

### 3. Test file: `control-plane/internal/httpapi/handlers/<resource>_test.go`

```go
package handlers

import (
    "bytes"
    "encoding/json"
    "log"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/parcel/control-plane/internal/store/memory"
)

func TestListWidgets_Empty(t *testing.T) {
    logger := log.New(&bytes.Buffer{}, "", 0)
    mem := memory.New()

    req := httptest.NewRequest(http.MethodGet, "/api/v1/widgets", nil)
    w := httptest.NewRecorder()

    ListWidgets(logger, mem, false).ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
    }
}
```

## Global-plane pattern

Same structure but:
- Handler goes in `control-plane/internal/globalplane/httpapi/handlers/<resource>.go`
- Route goes in `control-plane/internal/globalplane/httpapi/router.go`
- Store methods go in `control-plane/internal/globalplane/store.go` (interface) and a new `store_postgres_<resource>.go`
- Tests use the fake store pattern from `handlers/testutil_test.go` instead of `memory.New()`

## Reminders

- Always use a **narrow local interface** — never take the full `store.Store` as a parameter
- `trustProxy` controls whether to read real client IP from `X-Forwarded-For`; pass it through even if unused — it's a standard parameter pattern
- Return `[]T{}` (not `null`) for empty list responses
- Auth middleware is in the router, not the handler — handlers don't check roles directly
