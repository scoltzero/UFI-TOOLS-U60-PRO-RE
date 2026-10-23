// SPDX-License-Identifier: AGPL-3.0-or-later

package device

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrUnsupported = errors.New("device operation unsupported")

type Status struct {
	Model        string `json:"model"`
	Manufacturer string `json:"manufacturer"`
	IMEI         string `json:"imei,omitempty"`
	Signal       int    `json:"signal"`
	NetworkType  string `json:"network_type"`
	Connected    bool   `json:"connected"`
	SIMReady     bool   `json:"sim_ready"`
	Uptime       int64  `json:"uptime_seconds"`
}

type Device interface {
	Status(context.Context) (Status, error)
	GoFormGet(context.Context, string) (string, error)
	GoFormSet(context.Context, string, string) error
}

type MultiGoFormDevice interface {
	GoFormGetMany(context.Context, []string) (map[string]string, error)
}

// SnapshotDevice preserves structured status values (Wi-Fi access points,
// attached clients, and other arrays) instead of stringifying their JSON.
type SnapshotDevice interface {
	Snapshot(context.Context, []string) (map[string]any, error)
}

type GoFormCommandDevice interface {
	GoFormCommand(context.Context, map[string]string) error
}

type GoFormResultDevice interface {
	GoFormCommandResult(context.Context, map[string]string) (map[string]any, error)
}

type UBUSDevice interface {
	Call(context.Context, string, string, map[string]any) (map[string]any, error)
}

type TelemetryDevice interface {
	Telemetry(context.Context) (map[string]any, error)
}

func (s *Simulator) Telemetry(context.Context) (map[string]any, error) {
	return map[string]any{
		"simulated": true, "power_connected": true, "connections": map[string]any{"tcp": 0, "tcp_active": 0, "tcp_other": 0, "tcp6": 0, "udp": 0, "udp6": 0, "unix": 0}, "battery": 80, "battery_temperature": 32000,
		"cpu_temp": "42000", "cpu_usage": 12.5, "mem_usage": 35.0,
		"current_now": -150000, "voltage_now": 4100000,
		"internal_total_storage": int64(2 << 30), "internal_used_storage": int64(512 << 20),
		"internal_available_storage": int64(1536 << 20),
		"cpuFreqInfo":                map[string]any{"cpu0": map[string]int{"cur": 1200, "max": 2208}},
		"cpuUsageInfo":               map[string]float64{"cpu0": 12.5}, "cpu_temp_list": []int{42000},
		"memInfo": map[string]int{"mem_total_kb": 1048576, "mem_available_kb": 681574},
	}, nil
}

// Simulator is deterministic and never touches ADB, the LAN, or the host shell.
type Simulator struct {
	mu        sync.RWMutex
	started   time.Time
	values    map[string]string
	apns      map[string]map[string]any
	apnMode   int
	apnID     string
	apnNext   int
	wifi      []map[string]any
	blacklist []string
	messages  []map[string]any
	hostnames map[string]string
}

func NewSimulator() *Simulator {
	return &Simulator{
		started: time.Now(),
		values: map[string]string{
			"network_type": "5G",
			"signal":       "75",
			"sim_status":   "ready",
			"ppp_status":   "ppp_connected", "apn_mode": "auto", "network_signalbar": "4", "network_provider": "模拟网络",
			"monthly_tx_bytes": "1048576", "monthly_rx_bytes": "2097152", "wifi_access_sta_num": "0", "lan_ipaddr": "192.168.8.1", "lan_netmask": "255.255.255.0", "dhcpEnabled": "1", "dhcpStart": "192.168.8.100", "dhcpEnd": "192.168.8.200", "dhcpLease_hour": "12h",
		},
	}
}

func (s *Simulator) Status(context.Context) (Status, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Status{
		Model:        "U60 Pro (simulated)",
		Manufacturer: "ZTE",
		Signal:       75,
		NetworkType:  s.values["network_type"],
		Connected:    s.values["ppp_status"] == "ppp_connected",
		SIMReady:     true,
		Uptime:       int64(time.Since(s.started).Seconds()),
	}, nil
}

func (s *Simulator) GoFormGet(_ context.Context, key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.values[key]
	if !ok {
		return "", ErrUnsupported
	}
	return value, nil
}

func (s *Simulator) GoFormSet(_ context.Context, key, value string) error {
	if key == "" {
		return ErrUnsupported
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
	return nil
}

func (s *Simulator) GoFormGetMany(ctx context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		value, err := s.GoFormGet(ctx, key)
		if err == nil {
			result[key] = value
		}
	}
	return result, nil
}

func (s *Simulator) GoFormCommand(_ context.Context, values map[string]string) error {
	_, err := s.simulateCommand(values)
	return err
}
func (s *Simulator) GoFormCommandResult(_ context.Context, values map[string]string) (map[string]any, error) {
	return s.simulateCommand(values)
}

func GetMany(ctx context.Context, target Device, keys []string) (map[string]string, error) {
	if multi, ok := target.(MultiGoFormDevice); ok {
		return multi.GoFormGetMany(ctx, keys)
	}
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		value, err := target.GoFormGet(ctx, key)
		if err != nil {
			continue
		}
		result[key] = value
	}
	return result, nil
}

func Command(ctx context.Context, target Device, values map[string]string) error {
	if commander, ok := target.(GoFormCommandDevice); ok {
		return commander.GoFormCommand(ctx, values)
	}
	return ErrUnsupported
}

func CommandResult(ctx context.Context, target Device, values map[string]string) (map[string]any, error) {
	if resultDevice, ok := target.(GoFormResultDevice); ok {
		return resultDevice.GoFormCommandResult(ctx, values)
	}
	if err := Command(ctx, target, values); err != nil {
		return nil, err
	}
	return map[string]any{"result": "success"}, nil
}
