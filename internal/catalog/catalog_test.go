// SPDX-License-Identifier: AGPL-3.0-or-later

package catalog

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCatalogServer(t *testing.T) {
	root := t.TempDir()
	source := []byte(`<script>console.log("demo")</script>`)
	if err := os.WriteFile(filepath.Join(root, "demo.js"), source, 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"version":1,"plugins":[{"id":"demo","name":"Demo","version":"1.0.0","file":"demo.js","sha256":"","size":0}]}`
	if err := os.WriteFile(filepath.Join(root, "catalog.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	catalog, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(Handler(catalog, "secret"))
	defer server.Close()

	if response, _ := http.Get(server.URL + "/api/v1/plugins"); response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d", response.StatusCode)
	}
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/plugins/demo", nil)
	req.Header.Set("X-Store-Token", "secret")
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Digest") == "" {
		t.Fatalf("unexpected response: %d %#v", response.StatusCode, response.Header)
	}
}
