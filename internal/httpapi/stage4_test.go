package httpapi

import (
	"encoding/json"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/config"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/device"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/logfile"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/store"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func stageCall(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)
	return w
}
func TestAPNCRUDSelectionAndNoNestedCredentials(t *testing.T) {
	h := testRouter(t, "")
	body := `{"profile_name":"fixture","apn_wan_apn":"internet.fixture","apn_pdp_type":3,"apn_ppp_auth_mode":1,"apn_ppp_username":"synthetic-user","apn_ppp_passwd":"synthetic-password"}`
	for _, action := range []struct{ method, path, body string }{{"POST", "/api/apn/manual", body}, {"PUT", "/api/apn/manual/2", strings.Replace(body, "fixture", "edited", 1)}, {"POST", "/api/apn/manual/enable", `{"profileId":"2"}`}} {
		w := stageCall(h, action.method, action.path, action.body)
		if w.Code != 200 {
			t.Fatalf("%s %s: %d %s", action.method, action.path, w.Code, w.Body.String())
		}
	}
	for _, path := range []string{"/api/apn/all", "/api/apn/cid?cid=1"} {
		w := stageCall(h, "GET", path, "")
		if w.Code != 200 || strings.Contains(w.Body.String(), "synthetic-") {
			t.Fatalf("unsafe APN read %s", w.Body.String())
		}
	}
	w := stageCall(h, "GET", "/api/apn/all", "")
	if !strings.Contains(w.Body.String(), `"enabled_id":"2"`) || !strings.Contains(w.Body.String(), `"mode":"manual"`) {
		t.Fatal(w.Body.String())
	}
	if w = stageCall(h, "DELETE", "/api/apn/manual/2", ""); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w = stageCall(h, "POST", "/api/apn/manual/enable", `{"profileId":"2"}`); w.Code == 200 {
		t.Fatal("deleted profile selected")
	}
	parsed := apnPublic(map[string]any{"profileId": "1", "pdpType": map[string]any{"password": "nested-secret"}, "password": "secret"})
	encoded, _ := json.Marshal(parsed)
	if strings.Contains(string(encoded), "secret") {
		t.Fatal("nested raw field escaped whitelist")
	}
}
func TestHistoryDatesAndSchedulerContract(t *testing.T) {
	h := testRouter(t, "")
	start := time.Date(2026, 10, 21, 23, 59, 0, 0, time.UTC).Unix()
	for _, point := range []map[string]int64{{"ts": start, "total": 1000}, {"ts": start + 30, "total": 1100}, {"ts": start + 90, "total": 1300}} {
		raw, _ := json.Marshal(point)
		if w := stageCall(h, "POST", "/api/cellular/recent", string(raw)); w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	w := stageCall(h, "GET", "/api/cellularUsage?start=2026-10-21&end=2026-10-22", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"usage":100`) || !strings.Contains(w.Body.String(), `"usage":200`) || !strings.Contains(w.Body.String(), `"timezone":"UTC"`) {
		t.Fatal(w.Body.String())
	}
	if w := stageCall(h, "GET", "/api/cellularUsage?start=bad&end=2026-10-22", ""); w.Code != 400 {
		t.Fatal("invalid range accepted")
	}
	if w := stageCall(h, "POST", "/api/add_task", `{"id":"bad","data":{"enabled":true}}`); w.Code != 400 {
		t.Fatal("invalid scheduler accepted")
	}
	body := `{"id":"fixture","data":{"enabled":true,"next_unix":2000000000,"action":{"type":"goform_command","params":{"goformId":"DISCONNECT_NETWORK"}}}}`
	if w := stageCall(h, "POST", "/api/add_task", body); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = stageCall(h, "GET", "/api/list_tasks", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"id":"fixture"`) {
		t.Fatal(w.Body.String())
	}
	if w := stageCall(h, "DELETE", "/api/scheduler/fixture", ""); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
}
func TestSMSUIContractAndReadDelete(t *testing.T) {
	h := testRouter(t, "")
	path := "/api/goform/goform_get_cmd_process?cmd=sms_data_total&page=0&data_per_page=20"
	w := stageCall(h, "GET", path, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"content_encoding":"text"`) {
		t.Fatal(w.Body.String())
	}
	for _, cmd := range []string{`{"goformId":"SET_MSG_READ","msg_id":"1"}`, `{"goformId":"DELETE_SMS","msg_id":"1"}`, `{"goformId":"SEND_SMS","Number":"10000","MessageText":"local fixture only"}`} {
		if w := stageCall(h, "POST", "/api/goform/goform_set_cmd_process", cmd); w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	w = stageCall(h, "GET", path, "")
	if !strings.Contains(w.Body.String(), `"messages":[]`) {
		t.Fatal(w.Body.String())
	}
}
func TestRuntimeInactiveLogsAndTerminalCookieScope(t *testing.T) {
	db, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cfg := config.Defaults()
	cfg.PrivilegedCompat = true
	cfg.DeviceToken = "synthetic-terminal-token"
	logs := &logfile.File{Path: filepath.Join(t.TempDir(), "candidate.log")}
	_, _ = logs.Write([]byte("fixture-log"))
	h := New(Dependencies{Config: cfg, Store: db, Device: device.NewSimulator(), Logs: logs})
	call := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+cfg.DeviceToken)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	w := call("POST", "/api/time/config", `{"enabled":true,"server":"fixture.local","interval_hours":1}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"active":false`) {
		t.Fatal(w.Body.String())
	}
	if w = call("GET", "/api/logs", ""); w.Code != 200 || w.Body.String() != "fixture-log" {
		t.Fatal(w.Body.String())
	}
	if w = call("DELETE", "/api/logs", ""); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = call("POST", "/api/terminal/session", "")
	cookies := w.Result().Cookies()
	if w.Code != 200 || len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].Path != "/api/terminal/" {
		t.Fatalf("terminal session %s", w.Body.String())
	}
	r := httptest.NewRequest("GET", "/api/terminal/", nil)
	r.AddCookie(cookies[0])
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code == 401 {
		t.Fatal("terminal cookie not recognized")
	}
	r = httptest.NewRequest("GET", "/api/config", nil)
	r.AddCookie(cookies[0])
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("terminal cookie authorized unrelated API")
	}
}
func TestMalformedAPNListIsNotEmptySuccess(t *testing.T) {
	if _, err := apnProfiles(map[string]any{"unexpected": true}); err == nil {
		t.Fatal("unknown list silently empty")
	}
}
