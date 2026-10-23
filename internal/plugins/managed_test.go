// SPDX-License-Identifier: AGPL-3.0-or-later
package plugins

import (
	"strings"
	"testing"
)

func TestManagedPluginPreservesUserContentOrderAndDisabledChoice(t *testing.T) {
	manual := "<!-- user-owned -->\n<script>window.userOwned=1</script>"
	head, err := UpsertManaged(manual, "screen", "//<script>window.screen=1;//</script>")
	if err != nil {
		t.Fatal(err)
	}
	tail := "\n<style>.user-tail{color:red}</style>"
	head += tail
	for i := 0; i < 3; i++ {
		head, err = UpsertManaged(head, "screen", "<script>window.screen=2</script>")
		if err != nil {
			t.Fatal(err)
		}
	}
	if strings.Count(head, "[KANO_PLUGIN_START] ufi-store:screen") != 1 || !strings.HasPrefix(head, manual) || !strings.HasSuffix(head, tail) {
		t.Fatal("reinstall duplicated or reordered user content")
	}
	if !ManagedEnabled(head, "screen") {
		t.Fatal("installed plugin not active")
	}
	head = strings.Replace(head, "<script>window.screen=2</script>", "<!-- [kano_disabled]\n<script>window.screen=2</script>\n[kano_disabled] -->", 1)
	head, err = UpsertManaged(head, "screen", "<script>window.screen=3</script>")
	if err != nil {
		t.Fatal(err)
	}
	if ManagedEnabled(head, "screen") || !strings.Contains(head, "window.screen=3") {
		t.Fatal("update discarded disabled choice")
	}
	removed, err := RemoveManaged(head, "screen")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(removed, manual) || !strings.HasSuffix(removed, tail) || strings.Contains(removed, "window.screen") {
		t.Fatal("uninstall touched other content or retained plugin")
	}
	if _, err = UpsertManaged(manual, "../other", "<script></script>"); err == nil {
		t.Fatal("unsafe marker id accepted")
	}
	broken := "<!-- [KANO_PLUGIN_START] ufi-store:screen -->\n<script>window.user=1</script>"
	if _, err = RemoveManaged(broken, "screen"); err == nil {
		t.Fatal("deleted after incomplete marker")
	}
}
