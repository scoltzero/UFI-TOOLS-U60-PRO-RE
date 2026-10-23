package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/lanbundle"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/ztebackup"
)

func TestRunDefaultsToOfflinePreview(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	dir := filepath.Join(t.TempDir(), "plan")
	data := []byte("fixture")
	hash := lanbundle.Digest(data)
	plan := lanbundle.Plan{Schema: 1, ID: strings.Repeat("a", 32), DeploymentID: strings.Repeat("a", 32), Action: "install", BaselineSHA256: hash, CandidateSHA256: hash, RollbackSHA256: hash, BinarySHA256: hash}
	if err := lanbundle.Save(dir, plan, data, data); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	work := filepath.Join(t.TempDir(), "run")
	args := []string{"run", "--plan", dir, "--base", server.URL, "--run-dir", work, "--router-password-file", "does-not-exist"}
	if err := run(context.Background(), args, &out); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 || !strings.Contains(out.String(), "no login") {
		t.Fatal("preview was not offline")
	}
	if _, err := os.Stat(work); !os.IsNotExist(err) {
		t.Fatal("preview wrote execution files")
	}
	if err := run(context.Background(), append(args, "--execute"), &out); err == nil {
		t.Fatal("execute without acknowledgement accepted")
	}
	if calls.Load() != 0 {
		t.Fatal("incomplete execution flags contacted device")
	}
}

func TestPrepareCLIUsesBackupWithoutExposingContents(t *testing.T) {
	tmp := t.TempDir()
	bundle := filepath.Join(tmp, "bundle")
	os.Mkdir(bundle, 0o700)
	key := []byte("fixture-only-backup-key")
	rc := []byte("#!/bin/sh\n# private fixture marker\nexit 0\n")
	backup, err := (ztebackup.Builder{Password: key}).Build([]ztebackup.File{{Path: lanbundle.RCPath, Mode: 0o755, Data: rc}})
	if err != nil {
		t.Fatal(err)
	}
	baseline := filepath.Join(tmp, "baseline.bin")
	secret := filepath.Join(tmp, "key.txt")
	os.WriteFile(baseline, backup, 0o600)
	os.WriteFile(secret, key, 0o600)
	bin := make([]byte, 24)
	copy(bin, []byte{0x7f, 'E', 'L', 'F', 2, 1})
	bin[18] = 183
	files := map[string][]byte{"ufi-tools-u60pro": bin, "service.sh": []byte("# lan-bootstrap fixture\n")}
	var sums strings.Builder
	for name, data := range files {
		os.WriteFile(filepath.Join(bundle, name), data, 0o600)
		fmt.Fprintf(&sums, "%s  %s\n", lanbundle.Digest(data), name)
	}
	os.WriteFile(filepath.Join(bundle, "SHA256SUMS"), []byte(sums.String()), 0o600)
	dir := filepath.Join(tmp, "plan")
	var out bytes.Buffer
	if err := run(context.Background(), []string{"prepare", "--bundle", bundle, "--baseline-backup", baseline, "--backup-password-file", secret, "--out", dir}, &out); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := lanbundle.Load(dir); err != nil {
		t.Fatal(err)
	}
	token, err := os.ReadFile(filepath.Join(dir, "candidate-token"))
	if err != nil {
		t.Fatal(err)
	}
	if len(strings.TrimSpace(string(token))) != 64 || strings.Contains(out.String(), strings.TrimSpace(string(token))) {
		t.Fatal("generated token missing or printed")
	}
	info, _ := os.Stat(filepath.Join(dir, "candidate-token"))
	if info.Mode().Perm() != 0o600 {
		t.Fatal("generated token not private")
	}
	if strings.Contains(out.String(), string(key)) || strings.Contains(out.String(), "private fixture marker") {
		t.Fatal("private backup content leaked")
	}
}

func TestBackupCommandExportsOnlyAndDoesNotOverwrite(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/backup/back_parameter" {
			w.Write([]byte("encrypted-fixture"))
			return
		}
		var requests []struct {
			Params []json.RawMessage `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&requests)
		var method string
		json.Unmarshal(requests[0].Params[2], &method)
		switch method {
		case "web_login_info":
			fmt.Fprint(w, `[{"result":[0,{"zte_web_sault":"salt"}]}]`)
		case "web_login":
			fmt.Fprint(w, `[{"result":[0,{"result":"0","ubus_rpc_session":"11111111111111111111111111111111"}]}]`)
		case "device_backup_proc":
			fmt.Fprint(w, `[{"result":[0,{"result":"success"}]}]`)
		default:
			t.Errorf("unexpected operation: %s", method)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	tmp := t.TempDir()
	password := filepath.Join(tmp, "password")
	os.WriteFile(password, []byte("fixture-web-secret"), 0o600)
	destination := filepath.Join(tmp, "backup.bin")
	var out bytes.Buffer
	args := []string{"backup", "--base", server.URL, "--router-password-file", password, "--out", destination}
	if err := run(context.Background(), args, &out); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(destination)
	if info.Mode().Perm() != 0o600 {
		t.Fatal("backup file not private")
	}
	before := calls.Load()
	if err := run(context.Background(), args, &out); err == nil {
		t.Fatal("overwrote backup")
	}
	if before != calls.Load() {
		t.Fatal("existing output caused network activity")
	}
	if strings.Contains(out.String(), "fixture-web-secret") {
		t.Fatal("password printed")
	}
}
