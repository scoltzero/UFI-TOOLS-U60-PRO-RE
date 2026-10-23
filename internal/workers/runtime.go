// SPDX-License-Identifier: AGPL-3.0-or-later
package workers

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/device"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/smsforward"
	"strconv"
	"strings"
	"time"
)

// Runtime callbacks are supplied only by an explicitly enabled local host.
type Runtime struct {
	SyncTime  func(context.Context, string) error
	Update    func(context.Context, string) error
	ClearLogs func() error
}

func (m *Manager) runtimeTick(ctx context.Context) error {
	var failures []error
	for _, job := range []struct {
		key      string
		run      func(context.Context, string) error
		fallback int64
	}{
		{"time_config", m.runtime.SyncTime, 24}, {"auto_update_config", m.runtime.Update, 24}, {"log_clear", func(_ context.Context, _ string) error {
			if m.runtime.ClearLogs == nil {
				return errors.New("log manager unavailable")
			}
			return m.runtime.ClearLogs()
		}, 24},
	} {
		raw, err := m.store.Get(ctx, "kv", job.key)
		if err != nil {
			continue
		}
		var cfg struct {
			Enabled       bool   `json:"enabled"`
			IntervalHours int64  `json:"interval_hours"`
			Server        string `json:"server"`
			Mode          string `json:"mode"`
		}
		if json.Unmarshal([]byte(raw), &cfg) != nil || !cfg.Enabled {
			continue
		}
		interval := cfg.IntervalHours
		if interval < 1 {
			interval = job.fallback
		}
		previous, _ := m.store.Get(ctx, "kv", job.key+"_last_attempt")
		last, _ := strconv.ParseInt(previous, 10, 64)
		if m.now().Unix()-last < interval*3600 {
			continue
		}
		// Persist the attempt before any effect; failed jobs wait until next interval.
		if err = m.store.Set(ctx, "kv", job.key+"_last_attempt", strconv.FormatInt(m.now().Unix(), 10)); err != nil {
			return err
		}
		arg := cfg.Mode
		if job.key == "time_config" {
			arg = cfg.Server
		}
		if job.run == nil {
			err = errors.New("runtime operation is unavailable")
		} else {
			err = job.run(ctx, arg)
		}
		state := map[string]any{"last_unix": m.now().Unix(), "state": "success"}
		if err != nil {
			state["state"] = "failed"
			state["error"] = err.Error()
			failures = append(failures, err)
		}
		encoded, _ := json.Marshal(state)
		if e := m.store.Set(ctx, "kv", job.key+"_status", string(encoded)); e != nil {
			failures = append(failures, e)
		}
	}
	if err := m.powerTick(ctx); err != nil {
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}
func (m *Manager) powerTick(ctx context.Context) error {
	raw, err := m.store.Get(ctx, "kv", "power_status_forward_enabled")
	if err != nil {
		return nil
	}
	var cfg struct {
		Enabled string `json:"enabled"`
	}
	if json.Unmarshal([]byte(raw), &cfg) != nil || cfg.Enabled != "1" {
		return nil
	}
	source, ok := m.device.(device.TelemetryDevice)
	if !ok {
		return errors.New("power telemetry unavailable")
	}
	values, err := source.Telemetry(ctx)
	if err != nil {
		return err
	}
	state, ok := values["power_connected"].(bool)
	if !ok {
		return errors.New("power state unavailable")
	}
	next := strconv.FormatBool(state)
	before, err := m.store.Get(ctx, "kv", "power_status_previous")
	if err != nil {
		return m.store.Set(ctx, "kv", "power_status_previous", next)
	}
	if strings.TrimSpace(before) == next {
		return nil
	}
	body := "设备外部电源已断开"
	if state {
		body = "设备外部电源已连接"
	}
	if err = smsforward.New(m.store, m.device, time.Minute, nil, m.logger).Forward(ctx, smsforward.Message{From: "UFI candidate", Body: body, Date: m.now().UTC().Format(time.RFC3339)}); err != nil {
		return err
	}
	return m.store.Set(ctx, "kv", "power_status_previous", next)
}
func (m *Manager) SetRuntime(runtime Runtime) { m.runtime = runtime }
