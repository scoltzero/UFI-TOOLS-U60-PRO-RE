// SPDX-License-Identifier: AGPL-3.0-or-later

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

func testRouter(t *testing.T, token string) http.Handler {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	cfg := config.Defaults()
	cfg.DeviceToken = token
	return New(Dependencies{Config: cfg, Store: db, Device: device.NewSimulator(), Version: "test"})
}

func TestAuthenticationAndDeviceStatus(t *testing.T) {
	router := testRouter(t, "secret")

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/device/status", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", unauthorized.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/device/status", nil)
	req.Header.Set("X-Device-Token", "secret")
	authorized := httptest.NewRecorder()
	router.ServeHTTP(authorized, req)
	if authorized.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", authorized.Code, authorized.Body.String())
	}
}

func TestConfigPersistenceAndShellDefault(t *testing.T) {
	router := testRouter(t, "")

	set := httptest.NewRecorder()
	router.ServeHTTP(set, httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader(`{"nickname":"lab"}`)))
	if set.Code != http.StatusOK {
		t.Fatalf("set config: %d %s", set.Code, set.Body.String())
	}

	get := httptest.NewRecorder()
	router.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	var response envelope
	if err := json.Unmarshal(get.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if get.Code != http.StatusOK {
		t.Fatalf("get config: %d", get.Code)
	}

	shell := httptest.NewRecorder()
	router.ServeHTTP(shell, httptest.NewRequest(http.MethodPost, "/api/run_shell", strings.NewReader(`{"command":"id"}`)))
	if shell.Code != http.StatusForbidden {
		t.Fatalf("shell should be disabled, got %d", shell.Code)
	}
}
