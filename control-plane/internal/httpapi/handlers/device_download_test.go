package handlers

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/parcel/control-plane/internal/store/memory"
)

func checkinWithBody(t *testing.T, mem *memory.Store, deviceID string, body string, first bool) {
	t.Helper()
	logger := log.New(&bytes.Buffer{}, "", 0)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/checkin", strings.NewReader(body))
	if first {
		req, _ = attachMTLSDevice(t, mem, req, deviceID)
	} else {
		req = attachExistingMTLSDevice(t, mem, req, deviceID)
	}
	w := httptest.NewRecorder()
	DeviceCheckin(logger, mem, nil, false, "", nil, DeviceIdentityPolicy{}, nil, ArtifactSignaturePolicy{}).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("check-in: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func storedComponent(t *testing.T, mem *memory.Store, deviceID, name string) DeviceComponentState {
	t.Helper()
	st, ok, err := mem.GetDeviceState(deviceID)
	if err != nil || !ok {
		t.Fatalf("device state: ok=%v err=%v", ok, err)
	}
	return decodeDeviceComponents(st.ComponentsJSON)[name]
}

func TestCheckinStoresDownloadProgressAndClearsIt(t *testing.T) {
	mem := memory.New()
	id := uuid.NewString()
	withDL := `{"deviceId":"` + id + `","agentVersion":"0.1.0","current":{"softwareVersion":"v1","configRev":"c1"},` +
		`"currentComponents":{"app_bundle":{"softwareVersion":"v1","configRev":"c1","download":` +
		`{"artifactId":"a2","state":"downloading","bytesDone":4096,"bytesTotal":10000,"attempts":2,` +
		`"startedAt":"2026-11-10T10:00:00Z","lastProgressAt":"2026-11-10T10:05:00Z"}}}}`
	checkinWithBody(t, mem, id, withDL, true)

	d := storedComponent(t, mem, id, "app_bundle").Download
	if d == nil || d.State != "downloading" || d.BytesDone != 4096 || d.BytesTotal != 10000 || d.Attempts != 2 || d.ArtifactID != "a2" {
		t.Fatalf("download progress not stored: %+v", d)
	}

	// Next check-in without a download (finished or abandoned) clears it.
	without := `{"deviceId":"` + id + `","agentVersion":"0.1.0","current":{"softwareVersion":"v2","configRev":"c1"},` +
		`"currentComponents":{"app_bundle":{"softwareVersion":"v2","configRev":"c1"}}}`
	checkinWithBody(t, mem, id, without, false)
	if d := storedComponent(t, mem, id, "app_bundle").Download; d != nil {
		t.Fatalf("download should be cleared, got %+v", d)
	}
}

func TestDownloadingDeviceStaysActiveNotDegraded(t *testing.T) {
	mem := memory.New()
	id := uuid.NewString()
	body := `{"deviceId":"` + id + `","agentVersion":"0.1.0","current":{"softwareVersion":"v1","configRev":"c1"},` +
		`"currentComponents":{"app_bundle":{"softwareVersion":"v1","configRev":"c1","download":` +
		`{"artifactId":"a2","state":"waiting","reason":"stalled after 6 attempts","bytesDone":10,"bytesTotal":100}}}}`
	checkinWithBody(t, mem, id, body, true)
	dev, ok, _ := mem.GetDevice(id)
	if !ok || dev.Status != "active" {
		t.Fatalf("a deferred download must not degrade the device; status=%q ok=%v", dev.Status, ok)
	}
}

func TestSanitizeDownload(t *testing.T) {
	if sanitizeDownload(nil) != nil {
		t.Fatal("nil in, nil out")
	}
	if sanitizeDownload(&DeviceDownload{State: "exploding"}) != nil {
		t.Fatal("unknown state must be dropped")
	}
	got := sanitizeDownload(&DeviceDownload{
		ArtifactID: strings.Repeat("x", 500), State: "waiting",
		Reason: strings.Repeat("r", 1000), BytesDone: 500, BytesTotal: 100, Attempts: -3,
	})
	if got == nil {
		t.Fatal("valid state dropped")
	}
	if got.BytesDone != 100 || got.Attempts != 0 || len(got.Reason) != maxDownloadReasonLen || len(got.ArtifactID) != 128 {
		t.Fatalf("not clamped: %+v", got)
	}
	neg := sanitizeDownload(&DeviceDownload{State: "downloading", BytesDone: -5, BytesTotal: -1})
	if neg.BytesDone != 0 || neg.BytesTotal != 0 {
		t.Fatalf("negatives not clamped: %+v", neg)
	}
}
