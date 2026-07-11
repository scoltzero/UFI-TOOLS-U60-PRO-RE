// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/config"
)

func TestHealthAndVersion(t *testing.T) {
	cfg := config.Defaults()
	cfg.DataDir = t.TempDir()
	cfg.HTTPAddr = "127.0.0.1:0"
	a, err := New(cfg, WithVersion("test"))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	for _, test := range []struct {
		path string
		code int
	}{
		{"/healthz", http.StatusOK},
		{"/api/version", http.StatusOK},
	} {
		req := httptest.NewRequest(http.MethodGet, test.path, nil)
		recorder := httptest.NewRecorder()
		a.server.Handler.ServeHTTP(recorder, req)
		if recorder.Code != test.code {
			t.Fatalf("%s: got status %d", test.path, recorder.Code)
		}
	}
}

func TestNewGeneratesPersistentAuthenticationToken(t *testing.T) {
	cfg := config.Defaults()
	cfg.DataDir = t.TempDir()
	cfg.HTTPAddr = "127.0.0.1:0"
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if a.cfg.DeviceToken == "" {
		t.Fatal("expected generated token")
	}

	recorder := httptest.NewRecorder()
	a.server.Handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/need_token", nil))
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["need_token"] != true {
		t.Fatalf("expected token requirement: %s", recorder.Body.String())
	}

	second, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if second.cfg.DeviceToken != a.cfg.DeviceToken {
		t.Fatal("generated token must persist across restarts")
	}
}
