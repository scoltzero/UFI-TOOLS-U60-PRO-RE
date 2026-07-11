// SPDX-License-Identifier: AGPL-3.0-or-later

package update

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

func TestVerifyManifestAndArtifact(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	artifact := "test artifact"
	digest := sha256.Sum256([]byte(artifact))
	manifest := Manifest{
		Version: "0.0.1", Artifact: "ufi-tools-u60pro", SHA256: hex.EncodeToString(digest[:]),
		Size: int64(len(artifact)), Published: time.Now().UTC().Format(time.RFC3339),
	}
	payload, _ := manifest.payload()
	manifest.Signature = hex.EncodeToString(ed25519.Sign(privateKey, payload))
	if err := VerifyManifest(manifest, publicKey); err != nil {
		t.Fatal(err)
	}
	if err := VerifyArtifact(manifest, strings.NewReader(artifact)); err != nil {
		t.Fatal(err)
	}
	manifest.Version = "tampered"
	if err := VerifyManifest(manifest, publicKey); err == nil {
		t.Fatal("expected signature failure")
	}
}
