// SPDX-License-Identifier: AGPL-3.0-or-later

package laninstall

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestInstallDryRunAndAcknowledgement(t *testing.T) {
	client := &Client{}
	options := InstallOptions{Container: []byte("x"), BackupPath: filepath.Join(t.TempDir(), "backup"), HealthURL: "http://127.0.0.1/healthz"}
	if result, err := client.Install(context.Background(), options); err != nil || result.CandidateBytes != 1 {
		t.Fatalf("dry run = %#v, %v", result, err)
	}
	options.Execute = true
	if _, err := client.Install(context.Background(), options); err == nil {
		t.Fatal("expected acknowledgement error")
	}
	options.Acknowledged = true
	if _, err := client.Install(context.Background(), options); err == nil {
		t.Fatal("expected rollback-container error")
	}
}

func TestWaitRestartedHealthRequiresDownThenUp(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		switch calls.Add(1) {
		case 1:
			w.WriteHeader(http.StatusOK)
		case 2:
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()
	if !waitRestartedHealth(context.Background(), server.URL, time.Second, time.Millisecond) {
		t.Fatal("expected down/up transition to become healthy")
	}
	if calls.Load() < 3 {
		t.Fatalf("health accepted before restart transition: %d calls", calls.Load())
	}
}

func TestInstallSuccessfulWorkflow(t *testing.T) {
	var healthCalls atomic.Int32
	var restoreCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ubus/":
			body, _ := io.ReadAll(r.Body)
			if string(body) == "" {
				t.Fatal("empty RPC")
			}
			if restoreCalls.Add(1) > 0 {
				io.WriteString(w, `[{"result":[0,{"result":"success"}]}]`)
			}
		case "/backup/back_parameter":
			io.WriteString(w, "original")
		case "/cgi-bin/cgi-upload":
			io.WriteString(w, "checksum sha256sum")
		case "/healthz":
			if healthCalls.Add(1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
			} else {
				w.WriteHeader(http.StatusOK)
			}
		}
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	backupPath := filepath.Join(t.TempDir(), "original.bin")
	result, err := client.Install(context.Background(), InstallOptions{
		Container: []byte("candidate"), RollbackContainer: []byte("rollback"), BackupPath: backupPath, HealthURL: server.URL + "/healthz",
		Execute: true, Acknowledged: true, HealthTimeout: time.Second, RetryInterval: time.Millisecond,
	})
	if err != nil || !result.Healthy || !result.RestoreTriggered || result.RollbackAttempted {
		t.Fatalf("Install() = %#v, %v", result, err)
	}
}

func TestInstallWaitsForHealthyRollback(t *testing.T) {
	var restoreCalls atomic.Int32
	var healthCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ubus/":
			body, _ := io.ReadAll(r.Body)
			if len(body) == 0 {
				t.Fatal("empty RPC")
			}
			if string(body) != "" && containsProcedure(body, "device_restore_proc") {
				restoreCalls.Add(1)
			}
			io.WriteString(w, `[{"result":[0,{"result":"success"}]}]`)
		case "/cgi-bin/cgi-upload":
			io.WriteString(w, "checksum sha256sum")
		case "/healthz":
			call := healthCalls.Add(1)
			if restoreCalls.Load() < 2 || call%2 == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
			} else {
				w.WriteHeader(http.StatusOK)
			}
		}
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Install(context.Background(), InstallOptions{
		Container: []byte("candidate"), RollbackContainer: []byte("rollback"),
		BackupPath: filepath.Join(t.TempDir(), "rollback.bin"), HealthURL: server.URL + "/healthz",
		Execute: true, Acknowledged: true, HealthTimeout: 15 * time.Millisecond, RetryInterval: time.Millisecond,
	})
	if err == nil || !result.RollbackAttempted || !result.RollbackTriggered || !result.RollbackHealthy {
		t.Fatalf("Install() = %#v, %v", result, err)
	}
	if restoreCalls.Load() != 2 {
		t.Fatalf("restore calls = %d, want 2", restoreCalls.Load())
	}
}

func TestInstallReauthenticatesBeforeRollback(t *testing.T) {
	const firstSession = "11111111111111111111111111111111"
	const secondSession = "22222222222222222222222222222222"
	var loginCalls atomic.Int32
	var uploadCalls atomic.Int32
	var restoreCalls atomic.Int32
	var rollbackHealthCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ubus/":
			var calls []struct {
				Params []json.RawMessage `json:"params"`
			}
			if err := json.NewDecoder(r.Body).Decode(&calls); err != nil || len(calls) != 1 {
				t.Fatalf("invalid RPC: %v", err)
			}
			var procedure string
			if err := json.Unmarshal(calls[0].Params[2], &procedure); err != nil {
				t.Fatal(err)
			}
			switch procedure {
			case "web_login_info":
				io.WriteString(w, `[{"result":[0,{"zte_web_sault":"salt"}]}]`)
			case "web_login":
				call := loginCalls.Add(1)
				session := firstSession
				if call > 1 {
					session = secondSession
				}
				io.WriteString(w, `[{"result":[0,{"result":"0","ubus_rpc_session":"`+session+`"}]}]`)
			case "device_restore_proc":
				call := restoreCalls.Add(1)
				expected := firstSession
				if call > 1 {
					expected = secondSession
				}
				cookie, err := r.Cookie("webtoken")
				if err != nil || cookie.Value != expected {
					t.Fatalf("restore %d cookie = %#v, %v", call, cookie, err)
				}
				io.WriteString(w, `[{"result":[0,{"result":"success"}]}]`)
			default:
				t.Fatalf("unexpected procedure %q", procedure)
			}
		case "/cgi-bin/cgi-upload":
			call := uploadCalls.Add(1)
			expected := firstSession
			if call > 1 {
				expected = secondSession
			}
			cookie, err := r.Cookie("webtoken")
			if err != nil || cookie.Value != expected {
				t.Fatalf("upload %d cookie = %#v, %v", call, cookie, err)
			}
			io.WriteString(w, "checksum sha256sum")
		case "/healthz":
			if restoreCalls.Load() < 2 || rollbackHealthCalls.Add(1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Login(context.Background(), "router-secret"); err != nil {
		t.Fatal(err)
	}
	result, err := client.Install(context.Background(), InstallOptions{
		Container: []byte("candidate"), RollbackContainer: []byte("rollback"),
		BackupPath: filepath.Join(t.TempDir(), "rollback.bin"), HealthURL: server.URL + "/healthz",
		Execute: true, Acknowledged: true, HealthTimeout: 15 * time.Millisecond, RetryInterval: time.Millisecond,
	})
	if err == nil || !result.RollbackTriggered || !result.RollbackHealthy {
		t.Fatalf("Install() = %#v, %v", result, err)
	}
	if loginCalls.Load() != 2 {
		t.Fatalf("successful login calls = %d, want initial plus rollback reauthentication", loginCalls.Load())
	}
	if client.password != "" {
		t.Fatal("router password remained in the client after the install transaction")
	}
}

func containsProcedure(body []byte, procedure string) bool {
	return bytes.Contains(body, []byte(procedure))
}
