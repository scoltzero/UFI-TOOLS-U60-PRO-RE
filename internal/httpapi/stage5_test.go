// SPDX-License-Identifier: AGPL-3.0-or-later
package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/catalog"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/config"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/device"
	processrunner "github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/process"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/store"
)

func TestDevUIManagerShellContracts(t *testing.T) {
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cfg := config.Defaults()
	cfg.DeviceToken = "stage5-fixture-token"
	cfg.LegacyShellEnabled = true
	cfg.LegacyAuthEnabled = true
	runner := &fakeRunner{result: processrunner.Result{Output: "fixture output\n"}}
	router := New(Dependencies{Config: cfg, Store: db, Device: device.NewSimulator(), Runner: runner})
	for _, endpoint := range []string{"root_shell", "user_shell", "run_shell"} {
		t.Run(endpoint, func(t *testing.T) {
			// The real manager writes its two multiline support scripts in one call.
			command := "# DevUI support scripts\n" + strings.Repeat("# support-script content\n", 300)
			payload, _ := json.Marshal(map[string]string{"command": command})
			req := httptest.NewRequest("POST", "/api/"+endpoint, strings.NewReader(string(payload)))
			req.Header.Set("Authorization", "Bearer "+cfg.DeviceToken)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != 200 || runner.lastCommand != command {
				t.Fatalf("multiline script rejected: %d %s", rec.Code, rec.Body.String())
			}
			if endpoint == "run_shell" {
				var body struct {
					Code    int
					Success bool
					Content string
					Data    processrunner.Result
				}
				json.Unmarshal(rec.Body.Bytes(), &body)
				if !body.Success || body.Code != 0 || body.Content != runner.result.Output || body.Data.Output != body.Content {
					t.Fatalf("shim and envelope disagree: %s", rec.Body.String())
				}
			}
			runner.lastCommand = ""
			for _, withToken := range []bool{false, true} {
				payload, _ = json.Marshal(map[string]string{"command": strings.Repeat("x", maxShellCommandSize+1)})
				req = httptest.NewRequest("POST", "/api/"+endpoint, strings.NewReader(string(payload)))
				if withToken {
					req.Header.Set("Authorization", "Bearer "+cfg.DeviceToken)
				}
				rec = httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				expected := 401
				if withToken {
					expected = 400
				}
				if rec.Code != expected || runner.lastCommand != "" {
					t.Fatalf("invalid request ran: %d", rec.Code)
				}
			}
		})
	}
	for _, result := range []processrunner.Result{{Output: "command failed", ExitCode: 7}, {ExitCode: -1, TimedOut: true}} {
		runner.result = result
		req := httptest.NewRequest("POST", "/api/run_shell", strings.NewReader(`{"cmd":"fixture failure"}`))
		req.Header.Set("Authorization", "Bearer "+cfg.DeviceToken)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		var body struct {
			Code    int
			Success bool
		}
		json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != 500 || body.Success || body.Code != 500 {
			t.Fatalf("failure reported success: %s", rec.Body.String())
		}
	}
	cfg.LegacyShellEnabled = false
	router = New(Dependencies{Config: cfg, Store: db, Device: device.NewSimulator(), Runner: runner})
	for _, endpoint := range []string{"root_shell", "user_shell", "run_shell"} {
		runner.lastCommand = ""
		req := httptest.NewRequest("POST", "/api/"+endpoint, strings.NewReader(`{"command":"must not run"}`))
		req.Header.Set("Authorization", "Bearer "+cfg.DeviceToken)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != 403 || runner.lastCommand != "" {
			t.Fatalf("disabled shell executed: %s", endpoint)
		}
	}
}

// Browser integration is explicit because Playwright lives in the companion
// workspace. All commands go to this fixture runner, never a real shell/device.
func TestStage5BrowserIntegration(t *testing.T) {
	if os.Getenv("UFI_STAGE5_BROWSER") != "1" {
		t.Skip("run make stage5-integration for companion browser/data integration")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "shell-disabled", true: "shell-enabled"}[enabled], func(t *testing.T) {
			db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "state.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			cfg := config.Defaults()
			cfg.DeviceToken = "stage5-local-fixture-token"
			cfg.LegacyShellEnabled = enabled
			cfg.LegacyAuthEnabled = enabled
			runner := &devUIFixtureRunner{}
			var client *catalog.Client
			if enabled {
				workspace := os.Getenv("UFI_PLUGIN_WORKSPACE")
				if workspace == "" {
					workspace = filepath.Join(root, "..", "..", "ufitools_plugins")
				}
				source, err := os.ReadFile(filepath.Join(workspace, "plugins", "u60屏幕管理插件.txt"))
				if err != nil {
					t.Fatal(err)
				}
				dir := t.TempDir()
				if err = os.WriteFile(filepath.Join(dir, "screen.txt"), source, 0600); err != nil {
					t.Fatal(err)
				}
				manifest := `{"version":1,"plugins":[{"id":"screen-manager","name":"U60 Screen Manager","version":"local","file":"screen.txt"}]}`
				if err = os.WriteFile(filepath.Join(dir, "catalog.json"), []byte(manifest), 0600); err != nil {
					t.Fatal(err)
				}
				cat, err := catalog.Load(dir)
				if err != nil {
					t.Fatal(err)
				}
				catalogServer := httptest.NewServer(catalog.Handler(cat, ""))
				defer catalogServer.Close()
				client, err = catalog.NewClient(catalogServer.URL, "", nil)
				if err != nil {
					t.Fatal(err)
				}
			}
			server := httptest.NewServer(New(Dependencies{Config: cfg, Store: db, Device: device.NewSimulator(), Runner: runner, Catalog: client}))
			defer server.Close()
			mode := "disabled"
			if enabled {
				mode = "enabled"
			}
			command := exec.Command("node", filepath.Join(root, "scripts/test-stage5-browser.cjs"), server.URL, mode)
			output, err := command.CombinedOutput()
			t.Logf("%s", output)
			if err != nil {
				t.Fatal(err)
			}
			if !enabled {
				runner.mu.Lock()
				defer runner.mu.Unlock()
				if runner.shellCalls != 0 {
					t.Fatalf("disabled shell reached runner %d times", runner.shellCalls)
				}
			}
		})
	}
}

type devUIFixtureRunner struct {
	mu         sync.Mutex
	shellCalls int
}

func (f *devUIFixtureRunner) Run(_ context.Context, command string) (processrunner.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case strings.TrimSpace(command) == "id -u":
		f.shellCalls++
		return processrunner.Result{Output: "0\n"}, nil
	case strings.HasPrefix(strings.TrimSpace(command), "mkdir -p "):
		f.shellCalls++
		return processrunner.Result{Output: "ok\n"}, nil
	case strings.Contains(command, "INSTALLED_DEVUI="):
		f.shellCalls++
		return processrunner.Result{Output: "INSTALLED_DEVUI=yes\nINSTALLED_DATAD=yes\nSIZE_DEVUI=1000\nSIZE_DATAD=2000\nUI_FILES=4\nSUBPAGE_FILES=8\nFUNCTION_FILES=3\nRUNNING_DEVUI=1\nRUNNING_DATAD=1\nPERSIST=no\nLEGACY_RESIDUE=no\nLEGACY_ITEMS=\n"}, nil
	default:
		return processrunner.Result{Output: "unsupported fixture command", ExitCode: 1}, nil
	}
}
