// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestMigrateAndKeyValue(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.Set(ctx, "config", "nickname", "test-device"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, "config", "nickname")
	if err != nil {
		t.Fatal(err)
	}
	if got != "test-device" {
		t.Fatalf("got %q", got)
	}
	if _, err := s.Get(ctx, "unknown", "key"); err == nil {
		t.Fatal("expected unsupported table error")
	}
	if _, err := s.Get(ctx, "kv", "missing"); err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}
