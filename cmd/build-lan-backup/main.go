// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/ztebackup"
)

func main() {
	binaryPath := flag.String("binary", "", "signed ARM64 core binary")
	servicePath := flag.String("service", "installer/bundle/service.sh", "service script")
	rcLocalPath := flag.String("rc-local", "", "matching-firmware rc.local with the service start entry")
	passwordFile := flag.String("password-file", "", "file containing the authorized backup password")
	outputPath := flag.String("output", ztebackup.OutputName, "output backup file")
	ack := flag.Bool("acknowledge-unverified-format", false, "acknowledge that no U60 Pro restore fixture has validated this format")
	flag.Parse()
	if *binaryPath == "" || *rcLocalPath == "" || *passwordFile == "" || !*ack {
		fatal(fmt.Errorf("binary, rc-local, password-file, and acknowledge-unverified-format are required"))
	}
	binary := mustRead(*binaryPath)
	service := mustRead(*servicePath)
	rcLocal := mustRead(*rcLocalPath)
	password := mustRead(*passwordFile)
	for len(password) > 0 && (password[len(password)-1] == '\n' || password[len(password)-1] == '\r') {
		password = password[:len(password)-1]
	}
	backup, err := (ztebackup.Builder{Password: password}).Build([]ztebackup.File{
		{Path: "data/etc/rc.local", Mode: 0o755, Data: rcLocal},
		{Path: "data/data/ufi-tools/service.sh", Mode: 0o755, Data: service},
		{Path: "data/data/ufi-tools/ufi-tools-u60pro", Mode: 0o755, Data: binary},
	})
	fatal(err)
	fatal(os.WriteFile(*outputPath, backup, 0o600))
	fmt.Printf("created %s (%d bytes); upload/restore was not executed\n", *outputPath, len(backup))
}

func mustRead(path string) []byte {
	data, err := os.ReadFile(path)
	fatal(err)
	return data
}

func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
