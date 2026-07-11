// SPDX-License-Identifier: AGPL-3.0-or-later

package plugins

import "testing"

func TestParseSupportedFormats(t *testing.T) {
	tests := []struct {
		source string
		format Format
	}{
		{`<script>console.log("ok")</script>`, FormatScript},
		{`//<script>console.log("old")</script>`, FormatComment},
		{"// KANO_PLUGIN_START\n// name: demo\n<script></script>\n// KANO_PLUGIN_END", FormatKano},
		{`<div class="kano_disabled"><script></script></div>`, FormatDisabled},
	}
	for _, test := range tests {
		doc, err := Parse(test.source)
		if err != nil {
			t.Fatalf("parse %q: %v", test.source, err)
		}
		if doc.Format != test.format {
			t.Fatalf("got %s, want %s", doc.Format, test.format)
		}
	}
}

func TestRejectUnknownAndOversize(t *testing.T) {
	if _, err := Parse("plain text"); err == nil {
		t.Fatal("expected unknown format error")
	}
}
