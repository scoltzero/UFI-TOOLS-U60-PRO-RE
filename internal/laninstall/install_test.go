// SPDX-License-Identifier: AGPL-3.0-or-later

package laninstall

import (
	"context"
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
		Container: []byte("candidate"), BackupPath: backupPath, HealthURL: server.URL + "/healthz",
		Execute: true, Acknowledged: true, HealthTimeout: time.Second, RetryInterval: time.Millisecond,
	})
	if err != nil || !result.Healthy || !result.RestoreTriggered || result.RollbackAttempted {
		t.Fatalf("Install() = %#v, %v", result, err)
	}
}
