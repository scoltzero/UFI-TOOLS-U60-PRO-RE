// SPDX-License-Identifier: AGPL-3.0-or-later

package process

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestSupervisorLifecycle(t *testing.T) {
	directory := t.TempDir()
	supervisor := &Supervisor{
		Binary: "/bin/sh", Args: []string{"-c", "sleep 10"},
		PIDFile: filepath.Join(directory, "process.pid"), LogFile: filepath.Join(directory, "process.log"),
	}
	started, err := supervisor.Start()
	if err != nil {
		t.Fatal(err)
	}
	if !started.Installed || !started.Running || started.PID <= 0 {
		t.Fatalf("unexpected start: %#v", started)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stopped, err := supervisor.Stop(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stopped.Running {
		t.Fatalf("still running: %#v", stopped)
	}
}
