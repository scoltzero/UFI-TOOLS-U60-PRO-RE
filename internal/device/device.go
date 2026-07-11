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
