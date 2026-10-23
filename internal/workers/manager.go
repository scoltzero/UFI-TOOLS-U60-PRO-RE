// SPDX-License-Identifier: AGPL-3.0-or-later

package workers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/smsforward"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/device"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/store"
)

type Config struct {
	SchedulerInterval time.Duration
	CellularInterval  time.Duration
}

type Manager struct {
	cfg     Config
	store   *store.Store
	device  device.Device
	logger  *slog.Logger
	runtime Runtime
	now     func() time.Time
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

type Schedule struct {
	Enabled         bool   `json:"enabled"`
	NextUnix        int64  `json:"next_unix"`
	IntervalSeconds int64  `json:"interval_seconds,omitempty"`
	Action          Action `json:"action"`
	LastError       string `json:"last_error,omitempty"`
	Status          string `json:"status,omitempty"`
	LastRunUnix     int64  `json:"last_run_unix,omitempty"`
}

type Action struct {
	Type   string            `json:"type"`
	Key    string            `json:"key"`
	Value  string            `json:"value"`
	Params map[string]string `json:"params,omitempty"`
}

func New(cfg Config, state *store.Store, target device.Device, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{cfg: cfg, store: state, device: target, logger: logger, now: time.Now}
}

func (m *Manager) Start(parent context.Context) {
	if m.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	m.cancel = cancel
	m.wg.Add(3)
	go m.loop(ctx, 30*time.Second, "runtime", m.runtimeTick)
	go m.loop(ctx, m.cfg.SchedulerInterval, "scheduler", m.runSchedules)
	go m.loop(ctx, m.cfg.CellularInterval, "cellular", m.sampleCellular)
}

func (m *Manager) Stop() {
	if m.cancel == nil {
		return
	}
	m.cancel()
	m.wg.Wait()
	m.cancel = nil
}

func (m *Manager) loop(ctx context.Context, interval time.Duration, name string, operation func(context.Context) error) {
	defer m.wg.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := operation(ctx); err != nil && !errors.Is(err, context.Canceled) {
				m.logger.Warn("background worker failed", "worker", name, "error", err)
			}
		}
	}
}

func (m *Manager) runSchedules(ctx context.Context) error {
	items, err := m.store.ListSchedulers(ctx)
	if err != nil {
		return err
	}
	now := m.now().Unix()
	for _, item := range items {
		var schedule Schedule
		if err := json.Unmarshal([]byte(item.Data), &schedule); err != nil || !schedule.Enabled || schedule.Status == "running" || schedule.NextUnix > now {
			continue
		}
		// Reserve this occurrence before any external side effect. If the
		// process dies, the persisted running state requires operator review.
		schedule.Status = "running"
		schedule.LastRunUnix = now
		if schedule.IntervalSeconds > 0 {
			schedule.NextUnix = now + schedule.IntervalSeconds
		} else {
			schedule.Enabled = false
		}
		claimed, _ := json.Marshal(schedule)
		ok, err := m.store.CompareSwapScheduler(ctx, item.ID, item.Data, string(claimed))
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		schedule.LastError = ""
		var runErr error
		switch schedule.Action.Type {
		case "goform_set":
			runErr = m.device.GoFormSet(ctx, schedule.Action.Key, schedule.Action.Value)
		case "goform_command":
			runErr = device.Command(ctx, m.device, schedule.Action.Params)
		case "forward_status":
			runErr = m.forwardStatus(ctx, "定时设备信息")
		default:
			runErr = errors.New("unsupported scheduler action")
		}
		schedule.Status = "success"
		if runErr != nil {
			schedule.Status = "failed"
			schedule.LastError = runErr.Error()
		}
		encoded, _ := json.Marshal(schedule)
		if _, err = m.store.CompareSwapScheduler(ctx, item.ID, string(claimed), string(encoded)); err != nil {
			return err
		}

	}
	return nil
}

func (m *Manager) sampleCellular(ctx context.Context) error {
	values, err := device.GetMany(ctx, m.device, []string{"monthly_tx_bytes", "monthly_rx_bytes"})
	if err != nil {
		return err
	}
	tx, e1 := strconv.ParseInt(values["monthly_tx_bytes"], 10, 64)
	rx, e2 := strconv.ParseInt(values["monthly_rx_bytes"], 10, 64)
	if e1 != nil || e2 != nil || tx < 0 || rx < 0 {
		return errors.New("complete nonnegative traffic counters required")
	}
	if err = m.store.RecordUsage(ctx, m.now().Unix(), tx+rx); err != nil {
		return err
	}
	return m.checkLimit(ctx)

}

func readInt(ctx context.Context, target device.Device, key string) (int64, error) {
	value, err := target.GoFormGet(ctx, key)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(value, 10, 64)
}

func (m *Manager) forwardStatus(ctx context.Context, reason string) error {
	status, err := m.device.Status(ctx)
	if err != nil {
		return err
	}
	body := fmt.Sprintf("%s: %s, 网络 %s, 连接 %t, 信号 %d", reason, status.Model, status.NetworkType, status.Connected, status.Signal)
	return smsforward.New(m.store, m.device, time.Minute, nil, m.logger).Forward(ctx, smsforward.Message{From: "UFI candidate", Body: body, Date: m.now().UTC().Format(time.RFC3339)})
}
func (m *Manager) checkLimit(ctx context.Context) error {
	raw, err := m.store.Get(ctx, "kv", "data_limit")
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var cfg struct {
		Enabled string  `json:"data_flow_limit_enabled"`
		Limit   float64 `json:"data_flow_max_limit"`
		Period  string  `json:"data_flow_check_daily_or_monthly"`
		Forward string  `json:"data_limit_status_forward_enabled"`
	}
	if err = json.Unmarshal([]byte(raw), &cfg); err != nil {
		return err
	}
	if cfg.Enabled != "1" || cfg.Limit <= 0 {
		return nil
	}
	now := m.now().UTC()
	start := now.Format("2006-01-02")
	if cfg.Period != "daily" {
		start = now.Format("2006-01") + "-01"
	}
	days, err := m.store.DailyUsage(ctx, start, now.Format("2006-01-02"))
	if err != nil {
		return err
	}
	var used int64
	for _, d := range days {
		used += d.Usage
	}
	var calibration struct {
		Period string `json:"period"`
		Offset int64  `json:"offset"`
	}
	if v, e := m.store.Get(ctx, "kv", "traffic_calibration"); e == nil {
		_ = json.Unmarshal([]byte(v), &calibration)
		if calibration.Period == start {
			used += calibration.Offset
		}
	}
	if float64(used) < cfg.Limit {
		return nil
	}
	state, err := m.device.Status(ctx)
	if err != nil {
		return err
	}
	if state.Connected {
		if err = device.Command(ctx, m.device, map[string]string{"goformId": "DISCONNECT_NETWORK"}); err != nil {
			return err
		}
	}
	if cfg.Forward == "1" {
		key := "data_limit_notice_" + start
		if _, err = m.store.Get(ctx, "kv", key); errors.Is(err, sql.ErrNoRows) {
			if err = m.forwardStatus(ctx, "流量已达到限额，蜂窝连接已断开"); err != nil {
				return err
			}
			return m.store.Set(ctx, "kv", key, "sent")
		} else if err != nil {
			return err
		}
	}
	return nil
}
