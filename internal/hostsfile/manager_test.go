// SPDX-License-Identifier: AGPL-3.0-or-later

package hostsfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyAndRestore(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "hosts")
	backup := filepath.Join(directory, "state", "hosts.original")
	if err := os.WriteFile(target, []byte("127.0.0.1 localhost\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{Target: target, Backup: backup}
	if err := manager.Apply("127.0.0.1 localhost\n10.0.0.2 example.local\n"); err != nil {
		t.Fatal(err)
	}
	if err := manager.Restore(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(target)
	if string(data) != "127.0.0.1 localhost\n" {
		t.Fatalf("unexpected restored content: %q", data)
	}
}

func TestRejectInvalidHosts(t *testing.T) {
	if err := validate("not-enough-fields\n"); err == nil {
		t.Fatal("expected validation failure")
	}
}
