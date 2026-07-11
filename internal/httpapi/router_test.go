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
	"time"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/config"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/device"
	processrunner "github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/process"
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

func TestRawLegacyAuthorizationToken(t *testing.T) {
	router := testRouter(t, "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/device/status", nil)
	req.Header.Set("Authorization", "secret")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected raw legacy token compatibility, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestEmbeddedUpstreamAndRecoveryFrontends(t *testing.T) {
	router := testRouter(t, "secret")
	for _, test := range []struct {
		path     string
		contains string
	}{
		{"/", "UFI-TOOLS-WEB"},
		{"/script/requests.js", "KANO_baseURL"},
		{"/lang/zh.json", "func_list"},
		{"/re-admin/", "UFI-TOOLS U60 Pro RE"},
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), test.contains) {
			t.Fatalf("GET %s: %d, missing %q", test.path, recorder.Code, test.contains)
		}
	}
}

func TestUpstreamFrontendResponseShapes(t *testing.T) {
	router := testRouter(t, "")
	for _, test := range []struct {
		method   string
		path     string
		body     string
		contains []string
	}{
		{http.MethodGet, "/api/version_info", "", []string{`"app_ver":"test"`, `"model":"U60 Pro (simulated)"`}},
		{http.MethodGet, "/api/baseDeviceInfo", "", []string{`"network_type":"5G"`, `"app_ver":"test"`}},
		{http.MethodGet, "/api/goform/goform_get_cmd_process?cmd=network_type,signal&multi_data=1", "", []string{`"network_type":"5G"`, `"signal":"75"`}},
		{http.MethodPost, "/api/goform/goform_set_cmd_process", `{"goformId":"TEST","value":"ok"}`, []string{`"result":"success"`}},
		{http.MethodGet, "/api/get_theme", "", []string{`"backgroundEnabled":"false"`}},
		{http.MethodGet, "/api/is_weak_token", "", []string{`"is_weak_token":true`}},
	} {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		if test.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s %s: %d %s", test.method, test.path, recorder.Code, recorder.Body.String())
		}
		for _, expected := range test.contains {
			if !strings.Contains(recorder.Body.String(), expected) {
				t.Errorf("%s %s missing %s: %s", test.method, test.path, expected, recorder.Body.String())
			}
		}
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

func TestStateCompatibilityRoutes(t *testing.T) {
	router := testRouter(t, "")
	requests := []struct {
		method string
		path   string
		body   string
		code   int
	}{
		{http.MethodPost, "/api/hosts", `{"content":"127.0.0.1 localhost"}`, http.StatusOK},
		{http.MethodGet, "/api/hosts", "", http.StatusOK},
		{http.MethodPost, "/api/scheduler", `{"id":"night","data":{"enabled":true}}`, http.StatusOK},
		{http.MethodGet, "/api/scheduler", "", http.StatusOK},
		{http.MethodPost, "/api/sms_forward_sms", `{"enabled":false}`, http.StatusOK},
		{http.MethodGet, "/api/sms_forward_sms", "", http.StatusOK},
		{http.MethodPost, "/api/cellular/recent", `{"ts":100,"total":4096}`, http.StatusOK},
		{http.MethodGet, "/api/cellular/recent", "", http.StatusOK},
	}
	for _, test := range requests {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(test.method, test.path, strings.NewReader(test.body)))
		if recorder.Code != test.code {
			t.Fatalf("%s %s: got %d: %s", test.method, test.path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestObservedCompatibilityPathsAreRegistered(t *testing.T) {
	router := testRouter(t, "")
	paths := []string{
		"/api/SELinux", "/api/accept_terms", "/api/adb/mode", "/api/adb/status", "/api/adb_alive",
		"/api/add_task", "/api/apn/all", "/api/apn/auto", "/api/apn/cid", "/api/apn/manual",
		"/api/apn/manual/enable", "/api/apn/manual/enabled", "/api/apn/mode", "/api/auto_update/check_now",
		"/api/auto_update/config", "/api/baseDeviceInfo", "/api/cellularUsage", "/api/check_update",
		"/api/clear_task", "/api/connInfo", "/api/delete_all_uploads_data", "/api/delete_img",
		"/api/device_id", "/api/device_uuid", "/api/download_update", "/api/download_update_status",
		"/api/get_custom_head", "/api/get_log_status", "/api/get_res_server", "/api/get_task",
		"/api/get_theme", "/api/goform/goform_get_cmd_process", "/api/goform/goform_set_cmd_process",
		"/api/hasTTYD", "/api/hosts", "/api/hosts_restore", "/api/install_update", "/api/is_weak_token",
		"/api/list_tasks", "/api/list_uploads", "/api/log_clear", "/api/plugin/changelog",
		"/api/plugin/download", "/api/plugin/install", "/api/plugin/list", "/api/plugin/notification/read",
		"/api/plugin/notifications", "/api/plugin/uninstall", "/api/plugin/update/check",
		"/api/plugin/update/download", "/api/plugins_store", "/api/power_status_forward_enabled",
		"/api/proxy/test", "/api/remove_task", "/api/run_shell", "/api/set_custom_head", "/api/set_log_status",
		"/api/set_nickname", "/api/set_res_server", "/api/set_theme", "/api/set_token",
		"/api/set_wakelock_status", "/api/sms_forward_blacklist", "/api/sms_forward_curl",
		"/api/sms_forward_dingtalk", "/api/sms_forward_enabled", "/api/sms_forward_mail",
		"/api/sms_forward_method", "/api/sms_forward_sms", "/api/speedtest", "/api/time/config",
		"/api/time/status", "/api/time/sync", "/api/ttyd/start", "/api/ttyd/status", "/api/ttyd/stop",
		"/api/update_admin_pwd", "/api/upload_file", "/api/upload_img", "/api/uploads/test",
		"/api/usb_status", "/api/version_info", "/api/wifi/qrcode",
	}
	for _, path := range paths {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodOptions, path, nil))
		if recorder.Code == http.StatusNotFound {
			t.Errorf("observed path is not registered: %s", path)
		}
	}
}

func TestLegacyIdentitySettingsPersist(t *testing.T) {
	router := testRouter(t, "")
	for _, request := range []struct {
		path string
		body string
	}{
		{"/api/set_nickname", `{"nickname":"portable-lab"}`},
		{"/api/set_token", `{"token":"0123456789abcdef0123456789abcdef"}`},
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, request.path, strings.NewReader(request.body)))
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", request.path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestExplicitLegacyShellRunner(t *testing.T) {
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cfg := config.Defaults()
	cfg.DeviceToken = "0123456789abcdef"
	cfg.LegacyAuthEnabled = true
	cfg.LegacyShellEnabled = true
	router := New(Dependencies{
		Config: cfg, Store: db, Device: device.NewSimulator(),
		Runner: processrunner.LocalRunner{Timeout: time.Second, MaxOutput: 1024},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/run_shell", strings.NewReader(`{"command":"printf compatible"}`))
	req.Header.Set("X-Device-Token", cfg.DeviceToken)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "compatible") {
		t.Fatalf("unexpected shell response: %d %s", recorder.Code, recorder.Body.String())
	}
}

type fakeRunner struct {
	lastCommand string
	result      processrunner.Result
}

func (f *fakeRunner) Run(_ context.Context, command string) (processrunner.Result, error) {
	f.lastCommand = command
	return f.result, nil
}

func TestADBModeUsesFixedCommand(t *testing.T) {
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	runner := &fakeRunner{}
	router := New(Dependencies{Config: config.Defaults(), Store: db, Device: device.NewSimulator(), Runner: runner})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/adb/mode", strings.NewReader(`{"enabled":true,"port":5555}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected response: %d %s", recorder.Code, recorder.Body.String())
	}
	if runner.lastCommand != "setprop service.adb.tcp.port 5555; stop adbd; start adbd" {
		t.Fatalf("unexpected command: %q", runner.lastCommand)
	}
}

func TestAPNModeCompatibility(t *testing.T) {
	router := testRouter(t, "")
	set := httptest.NewRecorder()
	router.ServeHTTP(set, httptest.NewRequest(http.MethodPost, "/api/apn/mode", strings.NewReader(`{"mode":"manual","index":3}`)))
	if set.Code != http.StatusOK {
		t.Fatalf("set APN mode: %d %s", set.Code, set.Body.String())
	}
	get := httptest.NewRecorder()
	router.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/apn/mode", nil))
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), "manual") {
		t.Fatalf("get APN mode: %d %s", get.Code, get.Body.String())
	}
}
