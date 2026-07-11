// SPDX-License-Identifier: AGPL-3.0-or-later

package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
)

func TestLegacyImporterCreatesVerifiedSnapshot(t *testing.T) {
	source := []byte(`<script>console.log("legacy")</script>`)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/plugins":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": []any{map[string]any{
				"name": "demo.js", "public_name": "1.js", "size": len(source),
				"meta": map[string]any{"plugin_id": "abc123", "name": "Demo", "version": "1.0.0", "author": "Test"},
			}}})
		case "/api/v1/plugins/1.js/download":
			_, _ = w.Write(source)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	importer, err := NewLegacyImporter(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	output := t.TempDir() + "/snapshot"
	manifest, err := importer.Import(context.Background(), output)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Plugins) != 1 || manifest.Plugins[0].SHA256 == "" {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
	if _, err := os.Stat(output + "/catalog.json"); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(output); err != nil {
		t.Fatalf("snapshot should load: %v", err)
	}
}

func TestLegacyImporterRedirectRequiresExplicitHost(t *testing.T) {
	source := []byte(`<script>console.log("redirect")</script>`)
	download := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(source) }))
	defer download.Close()
	downloadURL, _ := url.Parse(download.URL)
	store := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/plugins":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": []any{map[string]any{
				"name": "demo.js", "public_name": "1.js", "size": len(source),
				"meta": map[string]any{"plugin_id": "abc123", "name": "Demo", "version": "1.0.0"},
			}}})
		case "/api/v1/plugins/1.js/download":
			http.Redirect(w, r, download.URL+"/plugin.js", http.StatusFound)
		}
	}))
	defer store.Close()
	client := store.Client()
	client.Transport = &http.Transport{TLSClientConfig: client.Transport.(*http.Transport).TLSClientConfig}

	importer, err := NewLegacyImporter(store.URL, client)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := importer.Import(context.Background(), t.TempDir()+"/denied"); err == nil {
		t.Fatal("expected redirected host to be denied")
	}
	importer, err = NewLegacyImporter(store.URL, client, downloadURL.Hostname())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := importer.Import(context.Background(), t.TempDir()+"/allowed"); err != nil {
		t.Fatal(err)
	}
}
