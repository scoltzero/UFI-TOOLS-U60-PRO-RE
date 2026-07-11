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

type GoFormCommandDevice interface {
	GoFormCommand(context.Context, map[string]string) error
}

type GoFormResultDevice interface {
	GoFormCommandResult(context.Context, map[string]string) (map[string]any, error)
}

// Simulator is deterministic and never touches ADB, the LAN, or the host shell.
type Simulator struct {
	mu      sync.RWMutex
	started time.Time
	values  map[string]string
}

func NewSimulator() *Simulator {
	return &Simulator{
		started: time.Now(),
		values: map[string]string{
			"network_type": "5G",
			"signal":       "75",
			"sim_status":   "ready",
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
		Connected:    true,
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
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, value := range values {
		if key != "goformId" && key != "isTest" {
			s.values[key] = value
		}
	}
	return nil
}

func (s *Simulator) GoFormCommandResult(ctx context.Context, values map[string]string) (map[string]any, error) {
	if err := s.GoFormCommand(ctx, values); err != nil {
		return nil, err
	}
	return map[string]any{"result": "success"}, nil
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
