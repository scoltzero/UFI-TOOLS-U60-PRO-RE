// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
)

func main() {
	privatePath := flag.String("private", "release-private-key.hex", "private key output")
	publicPath := flag.String("public", "release-public-key.hex", "public key output")
	flag.Parse()
	if _, err := os.Stat(*privatePath); err == nil {
		fatal(fmt.Errorf("refusing to overwrite %s", *privatePath))
	}
	if _, err := os.Stat(*publicPath); err == nil {
		fatal(fmt.Errorf("refusing to overwrite %s", *publicPath))
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	fatal(err)
	fatal(os.WriteFile(*privatePath, []byte(hex.EncodeToString(privateKey)+"\n"), 0o600))
	fatal(os.WriteFile(*publicPath, []byte(hex.EncodeToString(publicKey)+"\n"), 0o644))
	fmt.Printf("created %s and %s; keep the private key offline\n", *privatePath, *publicPath)
}

func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
