package client

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/parcel/agent/internal/httprange"
)

func TestGetArtifactChunks(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		_, _ = w.Write([]byte(`{"artifactId":"a1","sha256":"abc","size":10,"chunkSize":65536,"chunks":["h1","h2"]}`))
	}))
	defer srv.Close()

	m, err := New(srv.URL).GetArtifactChunks("a1", 65536)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/devices/artifacts/a1/chunks" || gotQuery != "chunkSize=65536" {
		t.Fatalf("unexpected request %s?%s", gotPath, gotQuery)
	}
	if m.SHA256 != "abc" || m.Size != 10 || m.ChunkSize != 65536 || len(m.Chunks) != 2 {
		t.Fatalf("bad manifest: %+v", m)
	}
}

func TestGetArtifactChunksStatusMapping(t *testing.T) {
	cases := []struct {
		status  int
		wantNil bool // true => must map to ErrNoChunkManifest
	}{
		{http.StatusNotFound, true},
		{http.StatusNotImplemented, true},
		{http.StatusInternalServerError, false},
		{http.StatusForbidden, false},
	}
	for _, c := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(c.status) }))
		_, err := New(srv.URL).GetArtifactChunks("a1", 65536)
		srv.Close()
		if err == nil {
			t.Fatalf("status %d: want error", c.status)
		}
		if got := errors.Is(err, httprange.ErrNoChunkManifest); got != c.wantNil {
			t.Errorf("status %d: ErrNoChunkManifest=%v, want %v (transient errors must stay distinguishable)", c.status, got, c.wantNil)
		}
	}
}
