// SPDX-License-Identifier: AGPL-3.0-or-later

package update

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

var safeVersion = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._+-]{0,63}$`)

type Manifest struct {
	Version   string `json:"version"`
	Artifact  string `json:"artifact"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
	Published string `json:"published"`
	Signature string `json:"signature"`
}

type unsignedManifest struct {
	Version   string `json:"version"`
	Artifact  string `json:"artifact"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
	Published string `json:"published"`
}

func (m Manifest) payload() ([]byte, error) {
	return json.Marshal(unsignedManifest{
		Version: m.Version, Artifact: m.Artifact, SHA256: strings.ToLower(m.SHA256), Size: m.Size, Published: m.Published,
	})
}

func (m Manifest) Validate() error {
	if !safeVersion.MatchString(m.Version) || m.Artifact == "" || m.Size <= 0 {
		return errors.New("manifest fields are incomplete")
	}
	if strings.Contains(m.Artifact, "/") || strings.Contains(m.Artifact, "\\") {
		return errors.New("artifact must be a base name")
	}
	digest, err := hex.DecodeString(m.SHA256)
	if err != nil || len(digest) != sha256.Size {
		return errors.New("invalid SHA-256 digest")
	}
	if _, err := time.Parse(time.RFC3339, m.Published); err != nil {
		return fmt.Errorf("invalid publication time: %w", err)
	}
	return nil
}

func VerifyManifest(m Manifest, publicKey ed25519.PublicKey) error {
	if err := m.Validate(); err != nil {
		return err
	}
	signature, err := hex.DecodeString(m.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return errors.New("invalid Ed25519 signature")
	}
	payload, err := m.payload()
	if err != nil {
		return err
	}
	if !ed25519.Verify(publicKey, payload, signature) {
		return errors.New("manifest signature verification failed")
	}
	return nil
}

func VerifyArtifact(m Manifest, reader io.Reader) error {
	hash := sha256.New()
	written, err := io.Copy(hash, io.LimitReader(reader, m.Size+1))
	if err != nil {
		return fmt.Errorf("hash artifact: %w", err)
	}
	if written != m.Size {
		return fmt.Errorf("artifact size mismatch: got %d, want %d", written, m.Size)
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), m.SHA256) {
		return errors.New("artifact digest mismatch")
	}
	return nil
}
