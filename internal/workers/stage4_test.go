package workers

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/device"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/store"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func workerFixture(t *testing.T) (*Manager, *store.Store, *device.Simulator) {
	t.Helper()
	db, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	d := device.NewSimulator()
	m := New(Config{}, db, d, nil)
	m.now = func() time.Time { return time.Date(2026, 10, 22, 1, 0, 0, 0, time.UTC) }
	return m, db, d
}
func TestRichTaskRunsOnceAndRunningClaimIsNotRepeated(t *testing.T) {
	m, db, d := workerFixture(t)
	schedule := Schedule{Enabled: true, NextUnix: 1, Action: Action{Type: "goform_command", Params: map[string]string{"goformId": "DISCONNECT_NETWORK"}}}
	raw, _ := json.Marshal(schedule)
	_ = db.SetScheduler(t.Context(), "once", string(raw))
	if err := m.runSchedules(t.Context()); err != nil {
		t.Fatal(err)
	}
	state, _ := d.Status(t.Context())
	if state.Connected {
		t.Fatal("rich task did not execute")
	}
	_ = device.Command(t.Context(), d, map[string]string{"goformId": "CONNECT_NETWORK"})
	if err := m.runSchedules(t.Context()); err != nil {
		t.Fatal(err)
	}
	state, _ = d.Status(t.Context())
	if !state.Connected {
		t.Fatal("one-shot repeated")
	}
	schedule.Enabled = true
	schedule.Status = "running"
	raw, _ = json.Marshal(schedule)
	_ = db.SetScheduler(t.Context(), "running", string(raw))
	if err := m.runSchedules(t.Context()); err != nil {
		t.Fatal(err)
	}
	state, _ = d.Status(t.Context())
	if !state.Connected {
		t.Fatal("unconfirmed prior effect repeated")
	}
}
func TestLimitUsesStoredDeltaAndActuallyDisconnects(t *testing.T) {
	m, db, d := workerFixture(t)
	now := m.now().Unix()
	_ = db.RecordUsage(t.Context(), now-60, 1000)
	_ = db.RecordUsage(t.Context(), now, 1200)
	_ = db.Set(t.Context(), "kv", "data_limit", `{"data_flow_limit_enabled":"1","data_flow_max_limit":150,"data_flow_check_daily_or_monthly":"daily"}`)
	if err := m.checkLimit(t.Context()); err != nil {
		t.Fatal(err)
	}
	state, _ := d.Status(t.Context())
	if state.Connected {
		t.Fatal("limit only saved; connection stayed up")
	}
}
func TestMissingTrafficCounterDoesNotInventZero(t *testing.T) {
	m, _, d := workerFixture(t)
	_ = d.GoFormSet(t.Context(), "monthly_tx_bytes", "100")
	_ = d.GoFormSet(t.Context(), "monthly_rx_bytes", "")
	if err := m.sampleCellular(t.Context()); err == nil {
		t.Fatal("partial counters accepted")
	}
}
func TestRuntimeExecutesConfiguredJobAndRecordsFailure(t *testing.T) {
	m, db, _ := workerFixture(t)
	_ = db.Set(t.Context(), "kv", "time_config", `{"enabled":true,"server":"local.fixture","interval_hours":1}`)
	calls := 0
	m.SetRuntime(Runtime{SyncTime: func(_ context.Context, server string) error {
		calls++
		if server != "local.fixture" {
			t.Fatal(server)
		}
		return errors.New("fixture synchronization failure")
	}})
	if err := m.runtimeTick(t.Context()); err == nil {
		t.Fatal("runtime failure hidden")
	}
	_ = m.runtimeTick(t.Context())
	if calls != 1 {
		t.Fatal("job retried before interval")
	}
	raw, _ := db.Get(t.Context(), "kv", "time_config_status")
	var state map[string]any
	_ = json.Unmarshal([]byte(raw), &state)
	if state["state"] != "failed" {
		t.Fatalf("runtime state %s", raw)
	}
}

type powerDevice struct {
	*device.Simulator
	plugged bool
}

func (d *powerDevice) Telemetry(context.Context) (map[string]any, error) {
	return map[string]any{"power_connected": d.plugged}, nil
}
func TestPowerChangesForwardOnceToLocalReceiver(t *testing.T) {
	m, db, d := workerFixture(t)
	sent := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { sent++; w.WriteHeader(204) }))
	defer server.Close()
	_ = db.Set(t.Context(), "kv", "power_status_forward_enabled", `{"enabled":"1"}`)
	_ = db.Set(t.Context(), "kv", "sms_forward_method", `"CURL"`)
	_ = db.Set(t.Context(), "kv", "sms_forward_curl", `{"url":"`+server.URL+`"}`)
	p := &powerDevice{Simulator: d, plugged: true}
	m.device = p
	if err := m.powerTick(t.Context()); err != nil {
		t.Fatal(err)
	}
	p.plugged = false
	if err := m.powerTick(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := m.powerTick(t.Context()); err != nil {
		t.Fatal(err)
	}
	if sent != 1 {
		t.Fatalf("power forwarded %d times", sent)
	}
}
