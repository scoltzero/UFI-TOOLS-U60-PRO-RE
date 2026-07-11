// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	updateverify "github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/update"
)

func main() {
	version := flag.String("version", "", "release version")
	artifactPath := flag.String("artifact", "", "artifact to sign")
	privateKeyPath := flag.String("private-key", "", "hex-encoded Ed25519 private key")
	outputPath := flag.String("output", "manifest.json", "manifest output path")
	flag.Parse()
	if *version == "" || *artifactPath == "" || *privateKeyPath == "" {
		fatalIf(fmt.Errorf("version, artifact, and private-key are required"))
	}

	artifact, err := os.ReadFile(*artifactPath)
	fatalIf(err)
	keyData, err := os.ReadFile(*privateKeyPath)
	fatalIf(err)
	privateKey, err := hex.DecodeString(strings.TrimSpace(string(keyData)))
	fatalIf(err)
	if len(privateKey) != ed25519.PrivateKeySize {
		fatalIf(fmt.Errorf("private key must be %d bytes", ed25519.PrivateKeySize))
	}

	digest := sha256.Sum256(artifact)
	manifest := updateverify.Manifest{
		Version: *version, Artifact: filepath.Base(*artifactPath), SHA256: hex.EncodeToString(digest[:]),
		Size: int64(len(artifact)), Published: time.Now().UTC().Format(time.RFC3339),
	}
	payload, err := json.Marshal(struct {
		Version   string `json:"version"`
		Artifact  string `json:"artifact"`
		SHA256    string `json:"sha256"`
		Size      int64  `json:"size"`
		Published string `json:"published"`
	}{manifest.Version, manifest.Artifact, manifest.SHA256, manifest.Size, manifest.Published})
	fatalIf(err)
	manifest.Signature = hex.EncodeToString(ed25519.Sign(ed25519.PrivateKey(privateKey), payload))
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	fatalIf(err)
	encoded = append(encoded, '\n')
	fatalIf(os.WriteFile(*outputPath, encoded, 0o644))
}

func fatalIf(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
