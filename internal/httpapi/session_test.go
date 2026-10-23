package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/config"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/device"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/store"
)

func TestCandidateTokenRotationIsImmediateAndPersistent(t *testing.T) {
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cfg := config.Defaults()
	cfg.DeviceToken = "initial-test-token"
	deps := Dependencies{Config: cfg, Store: db, Device: device.NewSimulator()}
	router := New(deps)
	call := func(h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := call(router, "POST", "/api/set_token", cfg.DeviceToken, `{"token":"rotated-valid-test-token"}`); w.Code != 200 {
		t.Fatalf("rotate: %d %s", w.Code, w.Body.String())
	}
	if w := call(router, "GET", "/api/config", cfg.DeviceToken, ""); w.Code != 401 {
		t.Fatalf("old token still works: %d", w.Code)
	}
	if w := call(router, "GET", "/api/config", "rotated-valid-test-token", ""); w.Code != 200 {
		t.Fatalf("new token rejected: %d", w.Code)
	}
	// New handler still receives the initial startup token, but stored rotation wins.
	if w := call(New(deps), "GET", "/api/config", "rotated-valid-test-token", ""); w.Code != 200 {
		t.Fatalf("rotation lost on restart: %d", w.Code)
	}
	if w := call(router, "POST", "/api/set_token", "rotated-valid-test-token", `{"token":"bad token with spaces"}`); w.Code != 400 {
		t.Fatalf("invalid header token accepted: %d", w.Code)
	}
	if w := call(router, "GET", "/api/config", "rotated-valid-test-token", ""); w.Code != 200 {
		t.Fatal("invalid rotation changed current token")
	}
	for _, invalid := range []string{"short77", "space 123", "line\n1234", strings.Repeat("x", 257)} {
		body, _ := json.Marshal(map[string]string{"token": invalid})
		if w := call(router, "POST", "/api/set_token", "rotated-valid-test-token", string(body)); w.Code != 400 {
			t.Fatal("invalid password accepted")
		}
	}
	// The user can set the same password as the factory UI without reading it.
	if w := call(router, "POST", "/api/set_token", "rotated-valid-test-token", `{"token":"Lab12345"}`); w.Code != 200 {
		t.Fatalf("eight-character password rejected: %d", w.Code)
	}
	if w := call(router, "GET", "/api/config", "rotated-valid-test-token", ""); w.Code != 401 {
		t.Fatal("previous credential still works")
	}
	reopened := New(deps)
	if w := call(reopened, "GET", "/api/config", "Lab12345", ""); w.Code != 200 {
		t.Fatal("eight-character rotation lost on reopen")
	}
	r := httptest.NewRequest("GET", "/api/capabilities", nil)
	r.Header.Set("X-Device-Token", "Lab12345")
	w := httptest.NewRecorder()
	reopened.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal("management UI does not share homepage credential")
	}
	db.Close()
	if w := call(router, "GET", "/api/config", "rotated-valid-test-token", ""); w.Code != 503 {
		t.Fatalf("auth storage failure must fail closed: %d", w.Code)
	}
}

func TestCandidateTelemetryHasUnitsAndSimulationLabel(t *testing.T) {
	w := httptest.NewRecorder()
	testRouter(t, "").ServeHTTP(w, httptest.NewRequest("GET", "/api/baseDeviceInfo", nil))
	var data map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || data["simulated"] != true || data["battery_temperature"] != float64(32000) || data["voltage_now"] != float64(4100000) || data["cpu_usage"] == nil {
		t.Fatalf("unexpected telemetry: %s", w.Body.String())
	}
}
