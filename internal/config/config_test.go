// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import "testing"

func TestDefaultsAreSafe(t *testing.T) {
	cfg := Defaults()
	if cfg.LegacyAuthEnabled || cfg.LegacyShellEnabled {
		t.Fatal("legacy compatibility must default to disabled")
	}
	if cfg.PluginServerURL != "" || cfg.ResourceServerURL != "" {
		t.Fatal("fresh installs must not contact an external control plane")
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("defaults should validate: %v", err)
	}
}

func TestShellRequiresLegacyAuthentication(t *testing.T) {
	cfg := Defaults()
	cfg.LegacyShellEnabled = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation failure")
	}
}
