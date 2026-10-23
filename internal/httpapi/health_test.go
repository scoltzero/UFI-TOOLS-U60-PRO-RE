package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/config"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/device"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/store"
)

func TestHealthIdentifiesProcessAndRequiresReadyStorage(t *testing.T) {
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	cfg := config.Defaults()
	cfg.DeviceToken = "private-auth-token"
	for _, instance := range []string{"", "test-instance-one", "test-instance-two"} {
		router := New(Dependencies{Config: cfg, Store: db, Device: device.NewSimulator(), HealthInstance: instance, HealthDeployment: "test-deployment"})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		var result struct {
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if rec.Code != 200 || result.Data["ok"] != true {
			t.Fatalf("health: %s", rec.Body.String())
		}
		if result.Data["deployment"] != "test-deployment" {
			t.Fatal("missing deployment receipt")
		}
		if instance == "" {
			if _, exists := result.Data["instance"]; exists {
				t.Fatal("unexpected instance")
			}
		} else if result.Data["instance"] != instance {
			t.Fatalf("wrong instance: %v", result.Data)
		}
	}
	db.Close()
	router := New(Dependencies{Config: cfg, Store: db, Device: device.NewSimulator(), HealthInstance: "not-ready"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != 503 {
		t.Fatalf("closed store reported healthy: %d", rec.Code)
	}
}
