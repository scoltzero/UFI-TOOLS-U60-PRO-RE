// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/ztebackup"
)

func main() {
	binaryPath := flag.String("binary", "", "ARM64 core binary")
	servicePath := flag.String("service", "installer/bundle/service.sh", "service script")
	sendATPath := flag.String("sendat", "bin/sendat-linux-arm64", "optional ARM64 AT helper; empty omits it")
	storeBinaryPath := flag.String("plugin-store-binary", "bin/ufi-plugin-store-linux-arm64", "optional ARM64 local plugin store; empty omits it")
	storeRootPath := flag.String("plugin-store-root", "plugin-store", "optional local plugin catalog directory; empty omits it")
	rcLocalPath := flag.String("rc-local", "", "matching-firmware rc.local with the service start entry")
	passwordFile := flag.String("password-file", "", "file containing the authorized backup password")
	imeiFile := flag.String("imei-file", "", "file containing the matching device IMEI; derives the firmware password")
	outputPath := flag.String("output", ztebackup.OutputName, "output backup file")
	ack := flag.Bool("acknowledge-unverified-format", false, "acknowledge that restoring the generated container mutates boot-critical device files")
	flag.Parse()
	if *binaryPath == "" || *rcLocalPath == "" || (*passwordFile == "" && *imeiFile == "") || !*ack {
		fatal(fmt.Errorf("binary, rc-local, password-file or imei-file, and acknowledge-unverified-format are required"))
	}
	binary := mustRead(*binaryPath)
	service := mustRead(*servicePath)
	rcLocal := mustRead(*rcLocalPath)
	var password []byte
	var imei string
	if *passwordFile != "" {
		password = mustRead(*passwordFile)
	} else {
		imei = string(mustRead(*imeiFile))
	}
	for len(password) > 0 && (password[len(password)-1] == '\n' || password[len(password)-1] == '\r') {
		password = password[:len(password)-1]
	}
	for len(imei) > 0 && (imei[len(imei)-1] == '\n' || imei[len(imei)-1] == '\r') {
		imei = imei[:len(imei)-1]
	}
	files := []ztebackup.File{
		{Path: "data/etc/rc.local", Mode: 0o755, Data: rcLocal},
		{Path: "data/data/ufi-tools/service.sh", Mode: 0o755, Data: service},
		{Path: "data/data/ufi-tools/ufi-tools-u60pro", Mode: 0o755, Data: binary},
	}
	if *sendATPath != "" {
		files = append(files, ztebackup.File{Path: "data/data/ufi-tools/sendat", Mode: 0o755, Data: mustRead(*sendATPath)})
	}
	if *storeBinaryPath != "" {
		files = append(files, ztebackup.File{Path: "data/data/ufi-tools/ufi-plugin-store", Mode: 0o755, Data: mustRead(*storeBinaryPath)})
	}
	if *storeRootPath != "" {
		files = append(files, mustReadTree(*storeRootPath, "data/data/ufi-tools/plugin-store")...)
	}
	backup, err := (ztebackup.Builder{Password: password, IMEI: imei}).Build(files)
	fatal(err)
	fatal(os.WriteFile(*outputPath, backup, 0o600))
	fmt.Printf("created %s (%d bytes); upload/restore was not executed\n", *outputPath, len(backup))
}

func mustReadTree(root, destination string) []ztebackup.File {
	var files []ztebackup.File
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("plugin store entry is not a regular file: %s", name)
		}
		relative, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		files = append(files, ztebackup.File{
			Path: filepath.ToSlash(filepath.Join(destination, relative)),
			Mode: int64(info.Mode().Perm()), Data: mustRead(name),
		})
		return nil
	})
	fatal(err)
	if len(files) == 0 {
		fatal(fmt.Errorf("plugin store directory is empty: %s", root))
	}
	return files
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
