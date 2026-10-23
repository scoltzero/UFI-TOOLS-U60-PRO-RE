package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/config"
)

func TestInstallerTokenSeedsOnceAndPreservesUserRotation(t *testing.T) {
	cfg := config.Defaults()
	cfg.DataDir = t.TempDir()
	cfg.HTTPAddr = "127.0.0.1:0"
	seed := filepath.Join(cfg.DataDir, "initial-device-token")
	os.WriteFile(seed, []byte(strings.Repeat("a", 64)), 0o600)
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if a.cfg.DeviceToken != strings.Repeat("a", 64) {
		t.Fatal("seed not applied")
	}
	if _, err := os.Stat(seed); !os.IsNotExist(err) {
		t.Fatal("seed not consumed")
	}
	if err := a.store.Set(context.Background(), "config", "device_token", strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	a.Close()
	os.WriteFile(seed, []byte(strings.Repeat("c", 64)), 0o600)
	b, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if b.cfg.DeviceToken != strings.Repeat("b", 64) {
		t.Fatal("installer replaced existing token")
	}
}

func TestInstallerTokenRejectsPublicFileOrSymlink(t *testing.T) {
	for _, kind := range []string{"public", "symlink", "invalid"} {
		t.Run(kind, func(t *testing.T) {
			cfg := config.Defaults()
			cfg.DataDir = t.TempDir()
			cfg.HTTPAddr = "127.0.0.1:0"
			seed := filepath.Join(cfg.DataDir, "initial-device-token")
			switch kind {
			case "public":
				os.WriteFile(seed, []byte(strings.Repeat("a", 64)), 0o644)
				os.Chmod(seed, 0o644)
			case "symlink":
				target := filepath.Join(t.TempDir(), "other")
				os.WriteFile(target, []byte(strings.Repeat("a", 64)), 0o600)
				os.Symlink(target, seed)
			case "invalid":
				os.WriteFile(seed, []byte("bad"), 0o600)
			}
			if a, err := New(cfg); err == nil {
				a.Close()
				t.Fatal("invalid seed accepted")
			}
		})
	}
}
