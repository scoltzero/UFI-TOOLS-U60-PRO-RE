// SPDX-License-Identifier: AGPL-3.0-or-later

package catalog

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestClientDownloadsVerifiedPlugin(t *testing.T) {
	root := t.TempDir()
	source := []byte(`<script>console.log("verified")</script>`)
	if err := os.WriteFile(filepath.Join(root, "demo.js"), source, 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"version":1,"plugins":[{"id":"demo","name":"Demo","version":"1","file":"demo.js","sha256":"","size":0}]}`
	if err := os.WriteFile(filepath.Join(root, "catalog.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(Handler(data, "token"))
	defer server.Close()
	client, err := NewClient(server.URL, "token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	item, downloaded, err := client.Download(context.Background(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if item.ID != "demo" || string(downloaded) != string(source) {
		t.Fatalf("unexpected download: %#v %q", item, downloaded)
	}
}
