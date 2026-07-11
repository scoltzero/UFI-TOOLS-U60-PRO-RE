// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	updateverify "github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/update"
)

func main() {
	manifestPath := flag.String("manifest", "manifest.json", "signed release manifest")
	artifactPath := flag.String("artifact", "ufi-tools-u60pro", "release artifact")
	publicKeyPath := flag.String("public-key", "public-key.hex", "trusted Ed25519 public key")
	flag.Parse()

	manifestData, err := os.ReadFile(*manifestPath)
	fatalIf(err)
	var manifest updateverify.Manifest
	fatalIf(json.Unmarshal(manifestData, &manifest))

	keyData, err := os.ReadFile(*publicKeyPath)
	fatalIf(err)
	publicKey, err := hex.DecodeString(strings.TrimSpace(string(keyData)))
	fatalIf(err)
	if len(publicKey) != ed25519.PublicKeySize {
		fatalIf(fmt.Errorf("public key must be %d bytes", ed25519.PublicKeySize))
	}
	fatalIf(updateverify.VerifyManifest(manifest, ed25519.PublicKey(publicKey)))

	artifact, err := os.Open(*artifactPath)
	fatalIf(err)
	defer artifact.Close()
	fatalIf(updateverify.VerifyArtifact(manifest, artifact))
}

func fatalIf(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
