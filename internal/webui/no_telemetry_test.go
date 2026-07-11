// SPDX-License-Identifier: AGPL-3.0-or-later

package webui

import (
	"io/fs"
	"strings"
	"testing"
)

func TestEmbeddedUIHasNoKnownImplicitExternalServices(t *testing.T) {
	forbidden := []string{
		"api.kanokano.cn/ufi_tools_report",
		"pan.kanokano.cn",
		"cg.163.com/#/mobile",
		"i.kano.ink",
		"t.alcy.cc/ycy",
	}
	err := fs.WalkDir(Files, "static", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, readErr := fs.ReadFile(Files, path)
		if readErr != nil {
			return readErr
		}
		for _, value := range forbidden {
			if strings.Contains(string(data), value) {
				t.Errorf("%s contains forbidden implicit external service %q", path, value)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestEmbeddedUIOverridesLegacyTermsWithProjectPolicy(t *testing.T) {
	data, err := fs.ReadFile(Files, "static/script/i18n.js")
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, required := range []string{"GNU AGPL-3.0-or-later", "does not send device identifiers", "不向作者或第三方发送设备标识"} {
		if !strings.Contains(content, required) {
			t.Errorf("project terms are missing %q", required)
		}
	}
}

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
