// SPDX-License-Identifier: AGPL-3.0-or-later

package pluginaudit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/catalog"
)

func TestCatalogExtractsLinksAndWarnings(t *testing.T) {
	root := t.TempDir()
	source := []byte(`<script>fetch("https://author.example/a");fetch("/api/root_shell"); eval("x")</script>`)
	digest := sha256.Sum256(source)
	manifest := catalog.Manifest{Version: 1, Plugins: []catalog.Plugin{{
		ID: "demo", Name: "Demo", Version: "1", File: "demo.js", Size: int64(len(source)), SHA256: hex.EncodeToString(digest[:]),
	}}}
	encoded, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(root, "catalog.json"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo.js"), source, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := catalog.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	findings, err := Catalog(loaded)
	if err != nil || len(findings) != 1 || len(findings[0].URLs) != 1 || len(findings[0].Warnings) != 2 {
		t.Fatalf("findings = %#v, %v", findings, err)
	}
}
