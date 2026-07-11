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
	"time"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/laninstall"
)

func main() {
	base := flag.String("base", "http://192.168.0.1", "U60 official web origin")
	containerPath := flag.String("container", "build/backup_restore.bin", "generated restore container")
	backupPath := flag.String("backup-output", "", "path for the original vendor backup")
	session := flag.String("ubus-session", "", "optional 32-character UBUS session")
	healthURL := flag.String("health-url", "", "candidate health URL (defaults to U60 host on port 2333)")
	execute := flag.Bool("execute", false, "perform backup, upload, and restore")
	ack := flag.Bool("acknowledge-device-restore", false, "acknowledge that restore reboots and mutates the device")
	healthTimeout := flag.Duration("health-timeout", 5*time.Minute, "candidate/rollback health window")
	flag.Parse()

	container, err := os.ReadFile(*containerPath)
	fatal(err)
	if *backupPath == "" {
		*backupPath = filepath.Join("build", "original-back_parameter-"+time.Now().UTC().Format("20060102T150405Z")+".bin")
	}
	if *healthURL == "" {
		parsed, err := url.Parse(*base)
		fatal(err)
		*healthURL = parsed.Scheme + "://" + net.JoinHostPort(parsed.Hostname(), "2333") + "/healthz"
	}
	client, err := laninstall.NewClient(*base, *session, nil)
	fatal(err)
	result, installErr := client.Install(context.Background(), laninstall.InstallOptions{
		Container: container, BackupPath: *backupPath, HealthURL: *healthURL,
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
