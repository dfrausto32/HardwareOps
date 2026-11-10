package artifacts

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestByteBudgetDefersThenResumes(t *testing.T) {
	payload, sum := testPayload(60_000)
	fs := &flakyServer{payload: payload, etag: `"v1"`}
	srv := httptest.NewServer(fs)
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "a.tar.gz")

	cfg := fastCfg()
	cfg.ByteBudget = 25_000
	var last int64
	for call := 1; call <= 2; call++ {
		_, err := downloadResumable(cfg, HTTPDownloader{srv.Client()}, srv.URL, dest, sum)
		var inc *DownloadIncompleteError
		if !errors.As(err, &inc) || inc.Reason != "byte budget reached" {
			t.Fatalf("call %d: want byte-budget incomplete, got %v", call, err)
		}
		if inc.Committed <= last || inc.Committed > int64(call)*25_000 {
			t.Fatalf("call %d: committed %d (prev %d)", call, inc.Committed, last)
		}
		if inc.Total != 60_000 {
			t.Fatalf("total %d", inc.Total)
		}
		last = inc.Committed
	}
	if _, err := downloadResumable(cfg, HTTPDownloader{srv.Client()}, srv.URL, dest, sum); err != nil {
		t.Fatalf("final call: %v", err)
	}
	b, _ := os.ReadFile(dest)
	if !bytes.Equal(b, payload) {
		t.Fatal("content mismatch")
	}
	// Each budgeted call must continue where the last stopped (a Range request),
	// not restart. (Server-side byte counts can't be used here: the server
	// writes ahead of what the budget-limited client consumes.)
	srv.Close()
	_, _, ranges := fs.snap()
	if len(ranges) != 3 || ranges[0] != "" || ranges[1] == "" || ranges[2] == "" {
		t.Fatalf("want 1 plain GET then 2 ranged GETs, got %q", ranges)
	}
}

func TestBudgetExactlyEqualToSizeCompletes(t *testing.T) {
	payload, sum := testPayload(10_000)
	srv := httptest.NewServer(&flakyServer{payload: payload, etag: `"v1"`})
	defer srv.Close()
	cfg := fastCfg()
	cfg.ByteBudget = 10_000
	dest := filepath.Join(t.TempDir(), "a.tar.gz")
	if _, err := downloadResumable(cfg, HTTPDownloader{srv.Client()}, srv.URL, dest, sum); err != nil {
		t.Fatalf("budget == size must complete, got %v", err)
	}
}

func TestBandwidthCapPacesDownload(t *testing.T) {
	payload, sum := testPayload(30_000)
	srv := httptest.NewServer(&flakyServer{payload: payload, etag: `"v1"`})
	defer srv.Close()
	cfg := fastCfg()
	cfg.MaxBytesPerSec = 100_000 // 30KB => ~300ms minimum
	dest := filepath.Join(t.TempDir(), "a.tar.gz")
	start := time.Now()
	if _, err := downloadResumable(cfg, HTTPDownloader{srv.Client()}, srv.URL, dest, sum); err != nil {
		t.Fatal(err)
	}
	if el := time.Since(start); el < 250*time.Millisecond {
		t.Fatalf("download finished in %v; cap of 100KB/s should take ~300ms", el)
	}
}

func TestStalledLinkReturnsIncompleteNotFailure(t *testing.T) {
	srv := httptest.NewServer(&flakyServer{status: http.StatusServiceUnavailable})
	defer srv.Close()
	cfg := fastCfg()
	cfg.MaxStalledAttempts = 2
	dest := filepath.Join(t.TempDir(), "a.tar.gz")
	_, err := downloadResumable(cfg, HTTPDownloader{srv.Client()}, srv.URL, dest, "")
	var inc *DownloadIncompleteError
	if !errors.As(err, &inc) {
		t.Fatalf("want DownloadIncompleteError, got %T %v", err, err)
	}
}

func TestPermanentErrorsAreNotIncomplete(t *testing.T) {
	srv := httptest.NewServer(&flakyServer{status: http.StatusNotFound})
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "a.tar.gz")
	_, err := downloadResumable(fastCfg(), HTTPDownloader{srv.Client()}, srv.URL, dest, "")
	var inc *DownloadIncompleteError
	if err == nil || errors.As(err, &inc) {
		t.Fatalf("404 must be a real failure, got %v", err)
	}
}

func TestOnProgressReportsMonotonicallyToTotal(t *testing.T) {
	payload, sum := testPayload(20_000)
	srv := httptest.NewServer(&flakyServer{payload: payload, etag: `"v1"`})
	defer srv.Close()
	cfg := fastCfg()
	var seen []int64
	var total int64
	cfg.OnProgress = func(c, tot int64) { seen = append(seen, c); total = tot }
	dest := filepath.Join(t.TempDir(), "a.tar.gz")
	if _, err := downloadResumable(cfg, HTTPDownloader{srv.Client()}, srv.URL, dest, sum); err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(seen); i++ {
		if seen[i] < seen[i-1] {
			t.Fatalf("progress went backwards: %v", seen)
		}
	}
	if len(seen) == 0 || seen[len(seen)-1] != 20_000 || total != 20_000 {
		t.Fatalf("final progress %v total %d", seen, total)
	}
}

func TestTransferWindow(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 1, 1, h, m, 0, 0, time.UTC) }
	var nilW *TransferWindow
	if !nilW.Open(at(3, 0)) {
		t.Fatal("nil window must be always open")
	}
	day, err := ParseTransferWindow("09:00-17:00")
	if err != nil {
		t.Fatal(err)
	}
	night, err := ParseTransferWindow("22:00-05:00")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		w    *TransferWindow
		t    time.Time
		want bool
	}{
		{day, at(9, 0), true}, {day, at(16, 59), true}, {day, at(17, 0), false}, {day, at(3, 0), false},
		{night, at(22, 0), true}, {night, at(23, 59), true}, {night, at(2, 0), true},
		{night, at(5, 0), false}, {night, at(12, 0), false},
	}
	for _, c := range cases {
		if got := c.w.Open(c.t); got != c.want {
			t.Errorf("%v open=%v want %v", c.t.Format("15:04"), got, c.want)
		}
	}
	for _, bad := range []string{"9-17", "09:00", "09:00-09:00", "25:00-26:00", "a-b"} {
		if _, err := ParseTransferWindow(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
	if w, err := ParseTransferWindow("  "); w != nil || err != nil {
		t.Errorf("empty must mean unrestricted, got %v %v", w, err)
	}
}
