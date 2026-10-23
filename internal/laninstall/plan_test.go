package laninstall

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/lanbundle"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/ztebackup"
)

func TestPlanWorkflowAndRecovery(t *testing.T) {
	for _, test := range []struct {
		name, action        string
		fail, changed, deny bool
	}{
		{"install", "install", false, false, false},
		{"uninstall", "uninstall", false, false, false},
		{"install-rollback", "install", true, false, false},
		{"uninstall-rollback", "uninstall", true, false, false},
		{"changed-baseline", "install", false, true, false},
		{"changed-mode", "install", false, true, false},
		{"occupied-port", "install", false, false, false},
		{"wrong-receipt", "uninstall", false, false, false},
		{"upload-denied", "install", false, false, false},
		{"login-denied", "install", false, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			key := []byte("fixture-backup-key")
			rc := []byte("#!/bin/sh\nexit 0\n")
			actualRC := rc
			if test.changed && test.name != "changed-mode" {
				actualRC = []byte("#!/bin/sh\n# new edit\nexit 0\n")
			}
			mode := int64(0o755)
			if test.name == "changed-mode" {
				mode = 0o750
			}
			backup, err := (ztebackup.Builder{Password: key}).Build([]ztebackup.File{{Path: lanbundle.RCPath, Mode: mode, Data: actualRC}})
			if err != nil {
				t.Fatal(err)
			}
			bundle := t.TempDir()
			binary := make([]byte, 24)
			copy(binary, []byte{0x7f, 'E', 'L', 'F', 2, 1})
			binary[18] = 183
			files := map[string][]byte{"ufi-tools-u60pro": binary, "service.sh": []byte("# lan-bootstrap fixture\n")}
			var sums strings.Builder
			for name, data := range files {
				os.WriteFile(filepath.Join(bundle, name), data, 0o600)
				fmt.Fprintf(&sums, "%s  %s\n", lanbundle.Digest(data), name)
			}
			os.WriteFile(filepath.Join(bundle, "SHA256SUMS"), []byte(sums.String()), 0o600)
			preparedBaseline, _ := (ztebackup.Builder{Password: key}).Build([]ztebackup.File{{Path: lanbundle.RCPath, Mode: 0o755, Data: rc}})
			p, candidate, rollback, err := lanbundle.Prepare(bundle, preparedBaseline, key, test.action, strings.Repeat("a", 32))
			if err != nil {
				t.Fatal(err)
			}
			id := p.DeploymentID

			var phase, logins, uploads, originalCalls atomic.Int32
			absent := func(w http.ResponseWriter) {
				conn, _, err := w.(http.Hijacker).Hijack()
				if err == nil {
					conn.Close()
				}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/ubus/":
					var request []struct {
						Params []json.RawMessage `json:"params"`
					}
					json.NewDecoder(r.Body).Decode(&request)
					var method string
					json.Unmarshal(request[0].Params[2], &method)
					switch method {
					case "web_login_info":
						io.WriteString(w, `[{"result":[0,{"zte_web_sault":"fixture-salt"}]}]`)
					case "web_login":
						if test.deny {
							io.WriteString(w, `[{"result":[0,{"result":"1"}]}]`)
							return
						}
						count := logins.Add(1)
						fmt.Fprintf(w, `[{"result":[0,{"result":"0","ubus_rpc_session":"%032x"}]}]`, count)
					case "device_backup_proc":
						io.WriteString(w, `[{"result":[0,{"result":"success"}]}]`)
					case "device_restore_proc":
						phase.Add(1)
						originalCalls.Store(0)
						io.WriteString(w, `[{"result":[0,{"result":"success"}]}]`)
					default:
						t.Errorf("unexpected method: %s", method)
					}
				case "/backup/back_parameter":
					if cookie, err := r.Cookie("webtoken"); err != nil || cookie.Value != fmt.Sprintf("%032x", logins.Load()) {
						t.Error("backup lacks authenticated cookie")
					}
					w.Write(backup)
				case "/cgi-bin/cgi-upload":
					if cookie, err := r.Cookie("webtoken"); err != nil || cookie.Value != fmt.Sprintf("%032x", logins.Load()) {
						t.Error("stale upload session")
					}
					uploads.Add(1)
					if test.name == "upload-denied" {
						w.WriteHeader(403)
						return
					}
					io.WriteString(w, "checksum=ok sha256sum=ok")
				case "/original":
					if phase.Load() > 0 && originalCalls.Add(1) == 1 {
						w.WriteHeader(503)
						return
					}
					io.WriteString(w, "ok")
				case "/candidate":
					ph := phase.Load()
					present := test.action == "uninstall" || test.name == "occupied-port"
					if ph == 1 && !test.fail {
						present = !present
					}
					if ph == 1 && test.fail {
						present = test.action == "uninstall"
					}
					if ph >= 2 {
						present = test.action == "uninstall"
					}
					if !present {
						absent(w)
						return
					}
					responseID := id
					if test.name == "wrong-receipt" {
						responseID = strings.Repeat("b", 32)
					}
					fmt.Fprintf(w, `{"data":{"ok":true,"instance":"instance-%d","deployment":"%s"}}`, ph, responseID)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "", server.Client())
			if err != nil {
				t.Fatal(err)
			}
			work := filepath.Join(t.TempDir(), "run")
			result, err := client.RunPlan(context.Background(), PlanOptions{Plan: p, Candidate: candidate, Rollback: rollback, BackupPassword: key, RouterPassword: "fixture-router-password", RunDir: work, CandidateURL: server.URL + "/candidate", OriginalURL: server.URL + "/original", Execute: true, Acknowledged: true, Timeout: 150 * time.Millisecond, Interval: time.Millisecond})
			if test.changed || test.deny || test.name == "occupied-port" || test.name == "wrong-receipt" {
				if err == nil || uploads.Load() != 0 || phase.Load() != 0 {
					t.Fatalf("unsafe preflight: %+v %v", result, err)
				}
			} else if test.name == "upload-denied" {
				if err == nil || phase.Load() != 0 {
					t.Fatal("restore was requested after failed upload")
				}
			} else if test.fail {
				if err == nil || !result.RollbackOutcomeObserved || phase.Load() != 2 || logins.Load() != 2 {
					t.Fatalf("rollback: %+v %v", result, err)
				}
			} else if err != nil || !result.OutcomeObserved || uploads.Load() != 1 {
				t.Fatalf("operation: %+v %v", result, err)
			}
			if client.password != "" {
				t.Fatal("password retained")
			}
			data, readErr := os.ReadFile(filepath.Join(work, "result.json"))
			if readErr != nil {
				t.Fatal(readErr)
			}
			if strings.Contains(string(data), "fixture-router-password") {
				t.Fatal("password in result")
			}
		})
	}
}

func TestPlanDryRunHasNoNetworkOrWrites(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	client, _ := NewClient(server.URL, "", server.Client())
	id := strings.Repeat("a", 32)
	data := []byte("x")
	hash := lanbundle.Digest(data)
	o := PlanOptions{Plan: lanbundle.Plan{Schema: 1, ID: id, Action: "install", DeploymentID: id, BaselineSHA256: hash, CandidateSHA256: hash, RollbackSHA256: hash, BinarySHA256: hash}, Candidate: data, Rollback: data, CandidateURL: server.URL + "/candidate", OriginalURL: server.URL + "/original", RunDir: filepath.Join(t.TempDir(), "not-created")}
	if _, err := client.RunPlan(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatal("dry run contacted device")
	}
	if _, err := os.Stat(o.RunDir); !os.IsNotExist(err) {
		t.Fatal("dry run created files")
	}
	o.CandidateURL = "http://example.com/healthz"
	if _, err := client.RunPlan(context.Background(), o); err == nil {
		t.Fatal("cross-host health accepted")
	}
}

func TestHealthDoesNotFollowRedirectsWithSuppliedClient(t *testing.T) {
	var calls atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer other.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL, 302) }))
	defer server.Close()
	client, err := NewClient(server.URL, "", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if observation := client.probe(context.Background(), server.URL); observation.healthy {
		t.Fatal("redirect accepted as healthy")
	}
	if calls.Load() != 0 {
		t.Fatal("health followed redirect")
	}
}
