// SPDX-License-Identifier: AGPL-3.0-or-later

package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type ProcessStatus struct {
	Installed bool `json:"installed"`
	Running   bool `json:"running"`
	PID       int  `json:"pid,omitempty"`
}

type Supervisor struct {
	Binary  string
	Args    []string
	PIDFile string
	LogFile string

	mu  sync.Mutex
	cmd *exec.Cmd
}

func (s *Supervisor) Status() ProcessStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked()
}

func (s *Supervisor) Start() (ProcessStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := s.statusLocked()
	if status.Running {
		return status, nil
	}
	if !status.Installed {
		return status, fmt.Errorf("binary not installed: %s", s.Binary)
	}
	if err := os.MkdirAll(filepathDir(s.PIDFile), 0o700); err != nil {
		return status, err
	}
	logFile, err := os.OpenFile(s.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return status, err
	}
	cmd := exec.Command(s.Binary, s.Args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return status, err
	}
	if err := os.WriteFile(s.PIDFile, []byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0o600); err != nil {
		_ = cmd.Process.Kill()
		logFile.Close()
		return status, err
	}
	s.cmd = cmd
	go func() {
		_ = cmd.Wait()
		_ = logFile.Close()
		s.mu.Lock()
		if s.cmd == cmd {
			s.cmd = nil
			_ = os.Remove(s.PIDFile)
		}
		s.mu.Unlock()
	}()
	return ProcessStatus{Installed: true, Running: true, PID: cmd.Process.Pid}, nil
}

func (s *Supervisor) Stop(ctx context.Context) (ProcessStatus, error) {
	s.mu.Lock()
	status := s.statusLocked()
	if !status.Running {
		s.mu.Unlock()
		return status, nil
	}
	process, err := os.FindProcess(status.PID)
	s.mu.Unlock()
	if err != nil {
		return status, err
	}
	_ = process.Signal(syscall.SIGTERM)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = process.Kill()
			return s.Status(), ctx.Err()
		case <-ticker.C:
			current := s.Status()
			if !current.Running {
				return current, nil
			}
		}
	}
}

func (s *Supervisor) statusLocked() ProcessStatus {
	_, binaryErr := os.Stat(s.Binary)
	status := ProcessStatus{Installed: binaryErr == nil}
	data, err := os.ReadFile(s.PIDFile)
	if err != nil {
		return status
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return status
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return status
	}
	if err := process.Signal(syscall.Signal(0)); err == nil || !errors.Is(err, os.ErrProcessDone) && !errors.Is(err, syscall.ESRCH) {
		status.Running = true
		status.PID = pid
	}
	return status
}

func filepathDir(path string) string {
	index := strings.LastIndexByte(path, '/')
	if index <= 0 {
		return "."
	}
	return path[:index]
}
