// SPDX-License-Identifier: AGPL-3.0-or-later

package update

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestManagerChecksAndStagesSignedUpdate(t *testing.T) {
	publicKey, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	artifact := []byte("signed-arm64-binary")
	digest := sha256.Sum256(artifact)
	manifest := Manifest{
		Version: "1.0.0", Artifact: "ufi-tools-u60pro", SHA256: hex.EncodeToString(digest[:]),
		Size: int64(len(artifact)), Published: time.Now().UTC().Format(time.RFC3339),
	}
	payload, _ := manifest.payload()
	manifest.Signature = hex.EncodeToString(ed25519.Sign(privateKey, payload))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/manifest.json" {
			_ = json.NewEncoder(w).Encode(manifest)
			return
		}
		if r.URL.Path == "/ufi-tools-u60pro" {
			_, _ = w.Write(artifact)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	root := t.TempDir()
	keyFile := filepath.Join(root, "trusted.hex")
	if err := os.WriteFile(keyFile, []byte(hex.EncodeToString(publicKey)), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"verify-release", "service.sh"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("helper"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	manager, err := NewManager(server.URL+"/manifest.json", keyFile, root, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	status, err := manager.Download(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "ready" {
		t.Fatalf("unexpected status: %#v", status)
	}
	staged, err := os.ReadFile(filepath.Join(root, "updates", "1.0.0", "ufi-tools-u60pro"))
	if err != nil || string(staged) != string(artifact) {
		t.Fatalf("unexpected staged artifact: %q %v", staged, err)
	}
}

func TestConcurrentBackgroundAndManualUpdateDoNotOverlap(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	key, _, _ := ed25519.GenerateKey(rand.Reader)
	root := t.TempDir()
	file := filepath.Join(root, "public.hex")
	_ = os.WriteFile(file, []byte(hex.EncodeToString(key)), 0600)
	manager, err := NewManager(server.URL+"/manifest.json", file, root, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := manager.Check(context.Background()); done <- err }()
	<-entered
	if _, err := manager.Download(context.Background()); err == nil {
		t.Error("concurrent update request was accepted")
	}
	close(release)
	if err := <-done; err == nil {
		t.Fatal("fixture error not reported")
	}
}
