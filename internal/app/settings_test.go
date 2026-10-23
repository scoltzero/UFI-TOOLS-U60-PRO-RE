package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/config"
)

// Reopen the real application and SQLite store, rather than reusing a handler
// that could accidentally retain settings only in memory. All peers are local.
func TestSettingsSaveRuntimeAndReopen(t *testing.T) {
	catalogCalls := 0
	catalog := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		catalogCalls++
		if r.URL.Path != "/api/v1/plugins" {
			t.Errorf("unexpected catalog request: %s", r.URL.Path)
		}
		io.WriteString(w, `{"version":1,"plugins":[]}`)
	}))
	defer catalog.Close()
	cfg := config.Defaults()
	cfg.DataDir, cfg.HTTPAddr = t.TempDir(), "127.0.0.1:0"
	cfg.DeviceToken = "stage31-synthetic-token"
	cfg.PluginServerURL, cfg.ResourceServerURL = catalog.URL, "https://startup.invalid"
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if a != nil {
			a.Close()
		}
	}()
	call := func(method, path, body string, status int) map[string]any {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+cfg.DeviceToken)
		w := httptest.NewRecorder()
		a.server.Handler.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		var value map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	data := func(path string) map[string]any { return call("GET", path, "", 200)["data"].(map[string]any) }
	if got := data("/api/config")["resource_server_url"]; got != cfg.ResourceServerURL {
		t.Fatal(got)
	}
	if got := data("/api/log_clear"); got["interval_hours"] != float64(24) || got["enabled"] != false || got["active"] != false {
		t.Fatal("missing setting does not expose worker defaults", got)
	}
	settings := map[string]string{
		"/api/set_nickname":       `{"nickname":"stage31 fixture"}`,
		"/api/set_res_server":     `{"res_server":"http://127.0.0.1:24881"}`,
		"/api/set_theme":          `{"themeColor":"180","backgroundEnabled":false,"backgroundUrl":""}`,
		"/api/time/config":        `{"enabled":false,"interval_hours":13}`,
		"/api/auto_update/config": `{"enabled":false,"interval_hours":19,"mode":"check"}`,
		"/api/log_clear":          `{"enabled":false,"interval_hours":37}`,
	}
	for path, body := range settings {
		call("POST", path, body, 200)
	}
	call("POST", "/api/log_clear", `{"enabled":false,"interval_hours":-1}`, 400)
	call("POST", "/api/time/config", `{"enabled":true,"server":"bad;server"}`, 400)
	call("POST", "/api/auto_update/config", `{"enabled":true}`, 409)
	verify := func() {
		t.Helper()
		got := data("/api/config")
		if got["nickname"] != "stage31 fixture" || got["resource_server_url"] != "http://127.0.0.1:24881" || got["plugin_server_url"] != catalog.URL {
			t.Fatal(got)
		}
		if call("GET", "/api/get_res_server", "", 200)["res_server"] != got["resource_server_url"] {
			t.Fatal("resource getters disagree")
		}
		if call("GET", "/api/get_theme", "", 200)["themeColor"] != "180" {
			t.Fatal("theme lost")
		}
		for path, hours := range map[string]float64{"/api/time/config": 13, "/api/auto_update/config": 19, "/api/log_clear": 37} {
			value := data(path)
			if value["enabled"] != false || value["active"] != false || value["interval_hours"] != hours {
				t.Fatalf("%s: %v", path, value)
			}
		}
		caps := data("/api/capabilities")
		if caps["workers"] != false || caps["device_writes"] != false || caps["at"] != false {
			t.Fatal(caps)
		}
	}
	verify()
	call("POST", "/api/set_log_status", `{"debug_log_enabled":false}`, 200)
	a.logger.Info("disabled-before-reopen")
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	a = nil
	a, err = New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	verify()
	if catalogCalls != 0 {
		t.Fatal("saving/reopening settings contacted catalog")
	}
	a.logger.Info("disabled-after-reopen")
	call("POST", "/api/set_log_status", `{"debug_log_enabled":true}`, 200)
	a.logger.Info("enabled-after-reopen")
	logs, err := os.ReadFile(filepath.Join(cfg.DataDir, "candidate.log"))
	if err != nil || strings.Contains(string(logs), "disabled-") || !strings.Contains(string(logs), "enabled-after-reopen") {
		t.Fatalf("log setting not applied: %s %v", logs, err)
	}
	call("POST", "/api/set_res_server", `{"res_server":""}`, 200)
	if got := data("/api/config")["resource_server_url"]; got != "" {
		t.Fatalf("clear restored startup value: %v", got)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	a = nil
	a, err = New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := data("/api/config")["resource_server_url"]; got != "" {
		t.Fatalf("cleared value lost after reopen: %v", got)
	}
	call("GET", "/api/store/plugins", "", 200)
	if catalogCalls != 1 {
		t.Fatal("configured plugin source not used on demand")
	}
}
