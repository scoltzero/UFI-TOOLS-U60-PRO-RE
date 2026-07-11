// SPDX-License-Identifier: AGPL-3.0-or-later

package hostsfile

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Manager struct {
	Target string
	Backup string
	mu     sync.Mutex
}

func (m *Manager) Apply(content string) error {
	if err := validate(content); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureBackup(); err != nil {
		return err
	}
	return atomicWrite(m.Target, []byte(content), 0o644)
}

func (m *Manager) Restore() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, err := os.ReadFile(m.Backup)
	if err != nil {
		return fmt.Errorf("read hosts backup: %w", err)
	}
	return atomicWrite(m.Target, data, 0o644)
}

func (m *Manager) ensureBackup() error {
	if _, err := os.Stat(m.Backup); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := os.ReadFile(m.Target)
	if err != nil {
		return fmt.Errorf("read original hosts: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(m.Backup), 0o700); err != nil {
		return err
	}
	return atomicWrite(m.Backup, data, 0o600)
}

func validate(content string) error {
	if len(content) > 256<<10 || strings.ContainsRune(content, '\x00') {
		return errors.New("hosts content is invalid or too large")
	}
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 || strings.ContainsAny(fields[0], "/\\") {
			return fmt.Errorf("invalid hosts line %q", line)
		}
	}
	return nil
}

func atomicWrite(target string, data []byte, mode os.FileMode) error {
	directory := filepath.Dir(target)
	temporary, err := os.CreateTemp(directory, ".ufi-hosts-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := bytes.NewReader(data).WriteTo(temporary); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, target)
}
