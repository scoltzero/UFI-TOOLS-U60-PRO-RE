// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvironmentAndFlagPrecedence(t *testing.T) {
	t.Setenv("UFI_DATA_DIR", t.TempDir())
	t.Setenv("UFI_HTTP_ADDR", "127.0.0.1:24881")
	t.Setenv("UFI_PLUGIN_SERVER_URL", "http://127.0.0.1:24882")
	t.Setenv("UFI_RESOURCE_SERVER_URL", "http://127.0.0.1:24883")
	t.Setenv("UFI_DEVICE_BACKEND", "simulator")
	t.Setenv("UFI_DEVICE_BASE_URL", "")
	t.Setenv("UFI_WORKERS_ENABLED", "false")
	t.Setenv("UFI_PRIVILEGED_COMPAT", "false")
	t.Setenv("UFI_AT_ENABLED", "false")
	before := os.Args
	t.Cleanup(func() { os.Args = before })
	os.Args = []string{"fixture"}
	env, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if env.HTTPAddr != "127.0.0.1:24881" || env.PluginServerURL != os.Getenv("UFI_PLUGIN_SERVER_URL") || env.ResourceServerURL != os.Getenv("UFI_RESOURCE_SERVER_URL") {
		t.Fatal("environment not applied")
	}
	flagDir := filepath.Join(t.TempDir(), "flag-data")
	os.Args = []string{"fixture", "--data-dir", flagDir, "--http-addr", "127.0.0.1:24884", "--resource-server-url", ""}
	flags, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if flags.DataDir != flagDir || flags.HTTPAddr != "127.0.0.1:24884" || flags.ResourceServerURL != "" || flags.PluginServerURL != env.PluginServerURL {
		t.Fatal("flag precedence or explicit clear failed")
	}
	for _, args := range [][]string{{"--data-dir", "relative"}, {"--http-addr", "bad"}, {"--plugin-server-url", "http://example.invalid"}, {"--device-backend", "zte-http"}} {
		os.Args = append([]string{"fixture"}, args...)
		if _, err := Load(); err == nil {
			t.Fatalf("invalid startup accepted: %v", args)
		}
	}
}

func TestDefaultsAreSafe(t *testing.T) {
	cfg := Defaults()
	if cfg.LegacyAuthEnabled || cfg.LegacyShellEnabled {
		t.Fatal("legacy compatibility must default to disabled")
	}
	if cfg.PluginServerURL != "" || cfg.ResourceServerURL != "" || cfg.UpdateManifestURL != "" || cfg.DeviceBaseURL != "" {
		t.Fatal("fresh installs must not contact an external control plane")
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("defaults should validate: %v", err)
	}
}

func TestZTEBackendRequiresExplicitURL(t *testing.T) {
	cfg := Defaults()
	cfg.DeviceBackend = "zte-http"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected missing URL failure")
	}
	cfg.DeviceBaseURL = "http://192.168.0.1"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected explicit backend to validate: %v", err)
	}
}

func TestShellRequiresLegacyAuthentication(t *testing.T) {
	cfg := Defaults()
	cfg.LegacyShellEnabled = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation failure")
	}
}
