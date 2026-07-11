// SPDX-License-Identifier: AGPL-3.0-or-later

package process

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"sync"
	"time"
)

type Result struct {
	Output    string `json:"output"`
	ExitCode  int    `json:"exit_code"`
	TimedOut  bool   `json:"timed_out"`
	Truncated bool   `json:"truncated"`
}

type Runner interface {
	Run(context.Context, string) (Result, error)
}

type LocalRunner struct {
	Shell     string
	Timeout   time.Duration
	MaxOutput int
}

func (r LocalRunner) Run(ctx context.Context, command string) (Result, error) {
	if command == "" {
		return Result{}, errors.New("command is empty")
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	maxOutput := r.MaxOutput
	if maxOutput <= 0 {
		maxOutput = 1 << 20
	}
	shell := r.Shell
	if shell == "" {
		shell = "/bin/sh"
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	buffer := &limitedBuffer{limit: maxOutput}
	cmd := exec.CommandContext(commandCtx, shell, "-c", command)
	cmd.Stdout = buffer
	cmd.Stderr = buffer
	err := cmd.Run()
	result := Result{Output: buffer.String(), Truncated: buffer.truncated}
	if errors.Is(commandCtx.Err(), context.DeadlineExceeded) {
		result.ExitCode = -1
		result.TimedOut = true
		return result, nil
	}
	if err == nil {
		return result, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.ExitCode = exitError.ExitCode()
		return result, nil
	}
	return result, err
}

type limitedBuffer struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	original := len(data)
	remaining := b.limit - b.buffer.Len()
	if remaining <= 0 {
		b.truncated = true
		return original, nil
	}
	if len(data) > remaining {
		data = data[:remaining]
		b.truncated = true
	}
	_, _ = b.buffer.Write(data)
	return original, nil
}

func (b *limitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}
