// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/laninstall"
)

func main() {
	base := flag.String("base", "http://192.168.98.1", "U60 official web origin")
	containerPath := flag.String("container", "build/backup_restore.bin", "generated restore container")
	backupPath := flag.String("backup-output", "", "local safety copy of the supplied rollback container")
	rollbackPath := flag.String("rollback-container", "", "verified container that restores the currently installed service and rc.local")
	legacyOriginalPath := flag.String("original-backup", "", "deprecated alias for --rollback-container")
	session := flag.String("ubus-session", "", "optional 32-character UBUS session")
	routerPasswordFile := flag.String("router-password-file", "", "file containing the router web password; logs in without exposing it in argv")
	healthURL := flag.String("health-url", "", "candidate health URL (defaults to U60 host on port 2333)")
	execute := flag.Bool("execute", false, "perform backup, upload, and restore")
	ack := flag.Bool("acknowledge-device-restore", false, "acknowledge that restore reboots and mutates the device")
	healthTimeout := flag.Duration("health-timeout", 5*time.Minute, "candidate/rollback health window")
	flag.Parse()

	container, err := os.ReadFile(*containerPath)
	fatal(err)
	if *backupPath == "" {
		*backupPath = filepath.Join("build", "rollback-copy-"+time.Now().UTC().Format("20060102T150405Z")+".bin")
	}
	if *healthURL == "" {
		parsed, err := url.Parse(*base)
		fatal(err)
		*healthURL = parsed.Scheme + "://" + net.JoinHostPort(parsed.Hostname(), "2333") + "/healthz"
	}
	client, err := laninstall.NewClient(*base, *session, nil)
	fatal(err)
	if *session != "" && *routerPasswordFile != "" {
		fatal(fmt.Errorf("use only one of --ubus-session or --router-password-file"))
	}
	if *execute && *session == "" && *routerPasswordFile == "" {
		fatal(fmt.Errorf("execute requires --router-password-file or --ubus-session"))
	}
	if *routerPasswordFile != "" && *execute {
		password, err := os.ReadFile(*routerPasswordFile)
		fatal(err)
		fatal(client.Login(context.Background(), strings.TrimRight(string(password), "\r\n")))
	}
	if *rollbackPath != "" && *legacyOriginalPath != "" {
		fatal(fmt.Errorf("use only one of --rollback-container or --original-backup"))
	}
	if *rollbackPath == "" {
		*rollbackPath = *legacyOriginalPath
	}
	var rollback []byte
	if *rollbackPath != "" {
		rollback, err = os.ReadFile(*rollbackPath)
		fatal(err)
	}
	result, installErr := client.Install(context.Background(), laninstall.InstallOptions{
		Container: container, RollbackContainer: rollback, BackupPath: *backupPath, HealthURL: *healthURL,
		Execute: *execute, Acknowledged: *ack, HealthTimeout: *healthTimeout,
	})
	encoded, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(encoded))
	if !*execute {
		fmt.Println("dry run only; add --execute --acknowledge-device-restore after validating the preview and rollback path")
	}
	fatal(installErr)
}

func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
