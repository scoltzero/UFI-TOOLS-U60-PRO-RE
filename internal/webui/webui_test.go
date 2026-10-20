// SPDX-License-Identifier: AGPL-3.0-or-later

package webui

import (
	"io/fs"
	"strings"
	"testing"
)

func TestEmbeddedUIIncludesUpstreamLicense(t *testing.T) {
	data, err := fs.ReadFile(Files, "static/UPSTREAM-LICENSE.txt")
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, required := range []string{"MIT License", "Copyright (c) [2025] [Minikano]", "UPSTREAM.lock"} {
		if !strings.Contains(content, required) {
			t.Errorf("upstream frontend attribution is missing %q", required)
		}
	}
}
