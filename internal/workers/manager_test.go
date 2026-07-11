// SPDX-License-Identifier: AGPL-3.0-or-later

package workers

import (
	"context"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/device"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/store"
)

func TestRunScheduleAndSampleCellular(t *testing.T) {
	ctx := context.Background()
	state, err := store.Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	target := device.NewSimulator()
	_ = target.GoFormSet(ctx, "monthly_tx_bytes", "100")
	_ = target.GoFormSet(ctx, "monthly_rx_bytes", "50")

	schedule := Schedule{Enabled: true, NextUnix: 99, Action: Action{Type: "goform_set", Key: "apn", Value: "internet"}}
	encoded, _ := json.Marshal(schedule)
	if err := state.SetScheduler(ctx, "once", string(encoded)); err != nil {
		t.Fatal(err)
	}
	manager := New(Config{SchedulerInterval: time.Second, CellularInterval: time.Second}, state, target, slog.Default())
	manager.now = func() time.Time { return time.Unix(100, 0) }
	if err := manager.runSchedules(ctx); err != nil {
		t.Fatal(err)
	}
	if got, _ := target.GoFormGet(ctx, "apn"); got != "internet" {
		t.Fatalf("scheduled action got %q", got)
	}
	if err := manager.sampleCellular(ctx); err != nil {
		t.Fatal(err)
	}
	points, err := state.RecentCellular(ctx, 1)
	if err != nil || len(points) != 1 || points[0].Total != 150 {
		t.Fatalf("unexpected points: %#v, %v", points, err)
	}
}

func TestManagerStartStop(t *testing.T) {
	state, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	manager := New(Config{SchedulerInterval: 5 * time.Millisecond, CellularInterval: 5 * time.Millisecond}, state, device.NewSimulator(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	manager.Start(ctx)
	cancel()
	manager.Stop()
}
