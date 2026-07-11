// SPDX-License-Identifier: AGPL-3.0-or-later

package workers

import (
	"context"
	"encoding/json"
	"errors"
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
	cfg    Config
	store  *store.Store
	device device.Device
	logger *slog.Logger
	now    func() time.Time
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type Schedule struct {
	Enabled         bool   `json:"enabled"`
	NextUnix        int64  `json:"next_unix"`
	IntervalSeconds int64  `json:"interval_seconds,omitempty"`
	Action          Action `json:"action"`
	LastError       string `json:"last_error,omitempty"`
}

type Action struct {
	Type  string `json:"type"`
	Key   string `json:"key"`
	Value string `json:"value"`
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
	m.wg.Add(2)
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
		if err := json.Unmarshal([]byte(item.Data), &schedule); err != nil || !schedule.Enabled || schedule.NextUnix > now {
			continue
		}
		if schedule.Action.Type != "goform_set" || schedule.Action.Key == "" {
			schedule.LastError = "unsupported scheduler action"
		} else if err := m.device.GoFormSet(ctx, schedule.Action.Key, schedule.Action.Value); err != nil {
			schedule.LastError = err.Error()
		} else {
			schedule.LastError = ""
		}
		if schedule.IntervalSeconds > 0 {
			schedule.NextUnix = now + schedule.IntervalSeconds
		} else {
			schedule.Enabled = false
		}
		encoded, err := json.Marshal(schedule)
		if err != nil {
			return err
		}
		if err := m.store.SetScheduler(ctx, item.ID, string(encoded)); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) sampleCellular(ctx context.Context) error {
	tx, txErr := readInt(ctx, m.device, "monthly_tx_bytes")
	rx, rxErr := readInt(ctx, m.device, "monthly_rx_bytes")
	if txErr != nil && rxErr != nil {
		return errors.Join(txErr, rxErr)
	}
	return m.store.RecordCellular(ctx, m.now().Unix(), tx+rx)
}

func readInt(ctx context.Context, target device.Device, key string) (int64, error) {
	value, err := target.GoFormGet(ctx, key)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(value, 10, 64)
}
