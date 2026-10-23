// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestCatalogPluginAtomicLifecycleAndReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "plugins.db")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { db.Close() }()
	manual := "<script>window.userOwned=1</script>"
	if err = db.Set(ctx, "kv", "custom_head", manual); err != nil {
		t.Fatal(err)
	}
	if err = db.SavePlugin(ctx, "screen", "<script>window.screen=1</script>", `{"id":"screen","version":"1"}`); err != nil {
		t.Fatal(err)
	}
	head, _ := db.Get(ctx, "kv", "custom_head")
	// A damaged user document must fail without removing the installed version.
	broken := strings.Replace(head, "<!-- [KANO_PLUGIN_END] ufi-store:screen -->", "", 1)
	if err = db.Set(ctx, "kv", "custom_head", broken); err != nil {
		t.Fatal(err)
	}
	if err = db.SavePlugin(ctx, "screen", "<script>window.screen=2</script>", `{"id":"screen","version":"2"}`); err == nil {
		t.Fatal("broken document accepted")
	}
	source, _ := db.Get(ctx, "kv", "plugin.screen")
	meta, _ := db.Get(ctx, "kv", "pluginmeta.screen")
	current, _ := db.Get(ctx, "kv", "custom_head")
	if !strings.Contains(source, "screen=1") || !strings.Contains(meta, `"version":"1"`) || current != broken {
		t.Fatal("failed installation was not atomic")
	}
	db.Set(ctx, "kv", "custom_head", head)
	db.Close()
	db, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	current, _ = db.Get(ctx, "kv", "custom_head")
	if current != head {
		t.Fatal("activation did not survive reopen")
	}
	if err = db.RemovePlugin(ctx, "screen"); err != nil {
		t.Fatal(err)
	}
	current, _ = db.Get(ctx, "kv", "custom_head")
	if !strings.Contains(current, manual) || strings.Contains(current, "window.screen") {
		t.Fatal("uninstall did not preserve user content")
	}
	for _, key := range []string{"plugin.screen", "pluginmeta.screen"} {
		if _, err = db.Get(ctx, "kv", key); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("uninstall retained %s", key)
		}
	}
}
