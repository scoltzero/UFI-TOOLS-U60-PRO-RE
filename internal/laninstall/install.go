// SPDX-License-Identifier: AGPL-3.0-or-later

package laninstall

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"
)

type InstallOptions struct {
	Container         []byte
	RollbackContainer []byte
	BackupPath        string
	HealthURL         string
	Execute           bool
	Acknowledged      bool
	HealthTimeout     time.Duration
	RetryInterval     time.Duration
}

type InstallResult struct {
	BackupPath        string `json:"backup_path"`
	RollbackBytes     int    `json:"rollback_bytes"`
	CandidateBytes    int    `json:"candidate_bytes"`
	RestoreTriggered  bool   `json:"restore_triggered"`
	Healthy           bool   `json:"healthy"`
	RollbackAttempted bool   `json:"rollback_attempted"`
	RollbackTriggered bool   `json:"rollback_triggered"`
	RollbackHealthy   bool   `json:"rollback_healthy"`
}

func (c *Client) Install(ctx context.Context, options InstallOptions) (InstallResult, error) {
	result := InstallResult{BackupPath: options.BackupPath, CandidateBytes: len(options.Container), RollbackBytes: len(options.RollbackContainer)}
	if len(options.Container) == 0 || options.BackupPath == "" || options.HealthURL == "" {
		return result, errors.New("container, backup path, and health URL are required")
	}
	if !options.Execute {
		return result, nil
	}
	if !options.Acknowledged {
		return result, errors.New("device restore acknowledgement is required")
	}
	if len(options.RollbackContainer) == 0 {
		return result, errors.New("a verified rollback container is required before device restore")
	}
	if len(options.RollbackContainer) > maxBackupSize {
		return result, errors.New("rollback container is too large")
	}
	if options.HealthTimeout <= 0 {
		options.HealthTimeout = 5 * time.Minute
	}
	if options.RetryInterval <= 0 {
		options.RetryInterval = 5 * time.Second
	}

	rollback := options.RollbackContainer
	if err := os.WriteFile(options.BackupPath, rollback, 0o600); err != nil {
		return result, fmt.Errorf("save rollback container: %w", err)
	}
	if err := c.Upload(ctx, options.Container); err != nil {
		return result, err
	}
	if err := c.Restore(ctx); err != nil {
		return result, err
	}
	result.RestoreTriggered = true
	if waitRestartedHealth(ctx, options.HealthURL, options.HealthTimeout, options.RetryInterval) {
		result.Healthy = true
		return result, nil
	}

	result.RollbackAttempted = true
	rollbackCtx, cancel := context.WithTimeout(context.Background(), options.HealthTimeout)
	defer cancel()
	if err := retry(rollbackCtx, options.RetryInterval, func() error { return c.Upload(rollbackCtx, rollback) }); err != nil {
		return result, fmt.Errorf("candidate health failed and rollback upload failed: %w", err)
	}
	if err := retry(rollbackCtx, options.RetryInterval, func() error { return c.Restore(rollbackCtx) }); err != nil {
		return result, fmt.Errorf("candidate health failed and rollback trigger failed: %w", err)
	}
	result.RollbackTriggered = true
	if !waitRestartedHealth(rollbackCtx, options.HealthURL, options.HealthTimeout, options.RetryInterval) {
		return result, errors.New("candidate health failed; rollback was triggered but did not recover health")
	}
	result.RollbackHealthy = true
	return result, errors.New("candidate health check failed; rollback restored a healthy service")
}

func waitRestartedHealth(ctx context.Context, target string, timeout, interval time.Duration) bool {
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	client := &http.Client{Timeout: 4 * time.Second}
	seenUnavailable := false
	for {
		request, err := http.NewRequestWithContext(checkCtx, http.MethodGet, target, nil)
		healthy := false
		if err == nil {
			response, requestErr := client.Do(request)
			if requestErr == nil {
				response.Body.Close()
				healthy = response.StatusCode >= 200 && response.StatusCode < 300
			}
		}
		if !healthy {
			seenUnavailable = true
		} else if seenUnavailable {
			return true
		}

		timer := time.NewTimer(interval)
		select {
		case <-checkCtx.Done():
			timer.Stop()
			return false
		case <-timer.C:
		}
	}
}

func retry(ctx context.Context, interval time.Duration, operation func() error) error {
	var last error
	for {
		if err := operation(); err == nil {
			return nil
		} else {
			last = err
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			if last != nil {
				return last
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}
