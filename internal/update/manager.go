// SPDX-License-Identifier: AGPL-3.0-or-later

package update

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Status struct {
	State    string `json:"state"`
	Version  string `json:"version,omitempty"`
	Artifact string `json:"artifact,omitempty"`
	Error    string `json:"error,omitempty"`
}

type Manager struct {
	manifestURL *url.URL
	publicKey   ed25519.PublicKey
	publicFile  string
	root        string
	client      *http.Client

	operation sync.Mutex
	mu        sync.RWMutex
	status    Status
	manifest  *Manifest
	stage     string
}

func NewManager(manifestURL, publicKeyFile, root string, client *http.Client) (*Manager, error) {
	parsed, err := url.Parse(manifestURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("invalid update manifest URL")
	}
	keyData, err := os.ReadFile(publicKeyFile)
	if err != nil {
		return nil, fmt.Errorf("read update public key: %w", err)
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(keyData)))
	if err != nil || len(key) != ed25519.PublicKeySize {
		return nil, errors.New("invalid update Ed25519 public key")
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	clientCopy := *client
	clientCopy.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != parsed.Scheme || !strings.EqualFold(req.URL.Host, parsed.Host) {
			return errors.New("update redirect changed origin")
		}
		if len(via) > 5 {
			return errors.New("too many update redirects")
		}
		return nil
	}
	return &Manager{
		manifestURL: parsed, publicKey: ed25519.PublicKey(key), publicFile: publicKeyFile,
		root: root, client: &clientCopy, status: Status{State: "idle"},
	}, nil
}

func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

func (m *Manager) Check(ctx context.Context) (Manifest, error) {
	if !m.operation.TryLock() {
		return Manifest{}, errors.New("update operation already running")
	}
	defer m.operation.Unlock()
	return m.check(ctx)
}
func (m *Manager) check(ctx context.Context) (Manifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.manifestURL.String(), nil)
	if err != nil {
		return Manifest{}, err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		m.fail(err)
		return Manifest{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("manifest server returned %d", resp.StatusCode)
		m.fail(err)
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&manifest); err != nil {
		m.fail(err)
		return Manifest{}, err
	}
	if manifest.Artifact != "ufi-tools-u60pro" {
		err := errors.New("update artifact must be named ufi-tools-u60pro")
		m.fail(err)
		return Manifest{}, err
	}
	if err := VerifyManifest(manifest, m.publicKey); err != nil {
		m.fail(err)
		return Manifest{}, err
	}
	m.mu.Lock()
	m.manifest = &manifest
	m.status = Status{State: "checked", Version: manifest.Version, Artifact: manifest.Artifact}
	m.mu.Unlock()
	return manifest, nil
}

func (m *Manager) Download(ctx context.Context) (Status, error) {
	if !m.operation.TryLock() {
		return m.Status(), errors.New("update operation already running")
	}
	defer m.operation.Unlock()
	manifest, err := m.check(ctx)
	if err != nil {
		return m.Status(), err
	}
	artifactURL := *m.manifestURL
	artifactURL.Path = filepath.ToSlash(filepath.Join(filepath.Dir(artifactURL.Path), manifest.Artifact))
	artifactURL.RawQuery = ""
	artifactURL.Fragment = ""
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, artifactURL.String(), nil)
	if err != nil {
		return m.Status(), err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		m.fail(err)
		return m.Status(), err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("artifact server returned %d", resp.StatusCode)
		m.fail(err)
		return m.Status(), err
	}
	stage := filepath.Join(m.root, "updates", manifest.Version)
	if err := os.MkdirAll(stage, 0o700); err != nil {
		m.fail(err)
		return m.Status(), err
	}
	temporary, err := os.CreateTemp(stage, ".download-")
	if err != nil {
		return m.Status(), err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := VerifyArtifact(manifest, io.TeeReader(resp.Body, temporary)); err != nil {
		temporary.Close()
		m.fail(err)
		return m.Status(), err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return m.Status(), err
	}
	if err := temporary.Close(); err != nil {
		return m.Status(), err
	}
	artifactPath := filepath.Join(stage, manifest.Artifact)
	if err := os.Rename(temporaryPath, artifactPath); err != nil {
		return m.Status(), err
	}
	manifestData, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(filepath.Join(stage, "manifest.json"), append(manifestData, '\n'), 0o600); err != nil {
		return m.Status(), err
	}
	if err := copyFile(m.publicFile, filepath.Join(stage, "public-key.hex"), 0o600); err != nil {
		return m.Status(), err
	}
	if err := copyFile(filepath.Join(m.root, "verify-release"), filepath.Join(stage, "verify-release"), 0o700); err != nil {
		return m.Status(), err
	}
	if err := copyFile(filepath.Join(m.root, "service.sh"), filepath.Join(stage, "service.sh"), 0o700); err != nil {
		return m.Status(), err
	}
	m.mu.Lock()
	m.stage = stage
	m.status = Status{State: "ready", Version: manifest.Version, Artifact: manifest.Artifact}
	m.mu.Unlock()
	return m.Status(), nil
}

func (m *Manager) Install(ctx context.Context) error {
	if !m.operation.TryLock() {
		return errors.New("update operation already running")
	}
	defer m.operation.Unlock()
	m.mu.RLock()
	stage := m.stage
	status := m.status
	m.mu.RUnlock()
	if status.State != "ready" || stage == "" {
		return errors.New("no verified update is ready")
	}
	m.mu.Lock()
	m.status.State = "installing"
	m.mu.Unlock()
	cmd := exec.CommandContext(ctx, filepath.Join(m.root, "service.sh"), "install", stage)
	if output, err := cmd.CombinedOutput(); err != nil {
		failure := fmt.Errorf("installer failed: %w: %s", err, strings.TrimSpace(string(output)))
		m.fail(failure)
		return failure
	}
	return nil
}

func (m *Manager) fail(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status.State = "error"
	m.status.Error = err.Error()
}

func copyFile(source, destination string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		return err
	}
	if err := output.Sync(); err != nil {
		output.Close()
		return err
	}
	return output.Close()
}
