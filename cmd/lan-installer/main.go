// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/lanbundle"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/laninstall"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/ztebackup"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func readKey(passwordFile, imeiFile string) ([]byte, error) {
	if (passwordFile == "") == (imeiFile == "") {
		return nil, errors.New("supply exactly one of --backup-password-file or --imei-file")
	}
	file := passwordFile
	if file == "" {
		file = imeiFile
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	value := strings.TrimRight(string(data), "\r\n")
	if imeiFile != "" {
		if len(value) != 15 || strings.Trim(value, "0123456789") != "" {
			return nil, errors.New("IMEI file must contain 15 digits")
		}
		return ztebackup.DevicePassword(value), nil
	}
	if value == "" {
		return nil, errors.New("backup password file is empty")
	}
	return []byte(value), nil
}

func run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(out, "       install-lan.sh backup --base http://192.168.98.1 --router-password-file FILE --out NEW_FILE")
		fmt.Fprintln(out, "usage: install-lan.sh prepare [--action install|uninstall] --bundle DIR --baseline-backup FILE --imei-file FILE --out NEW_DIR [--installed-plan DIR]")
		fmt.Fprintln(out, "       install-lan.sh run --plan DIR [--base http://192.168.98.1] [--execute --acknowledge-reboot --router-password-file FILE --imei-file FILE --run-dir NEW_DIR]")
		return nil
	}
	switch args[0] {
	case "backup":
		f := flag.NewFlagSet("backup", flag.ContinueOnError)
		f.SetOutput(out)
		base := f.String("base", "http://192.168.98.1", "official device web origin")
		passwordFile := f.String("router-password-file", "", "router web password file")
		output := f.String("out", "", "new private backup file")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if *passwordFile == "" || *output == "" {
			return errors.New("router-password-file and out are required")
		}
		if _, err := os.Lstat(*output); err == nil {
			return errors.New("backup output already exists")
		} else if !os.IsNotExist(err) {
			return err
		}
		client, err := laninstall.NewClient(*base, "", nil)
		if err != nil {
			return err
		}
		password, err := os.ReadFile(*passwordFile)
		if err != nil {
			return err
		}
		if err = client.Login(ctx, strings.TrimRight(string(password), "\r\n")); err != nil {
			return err
		}
		data, err := client.Backup(ctx)
		if err != nil {
			return err
		}
		if err = os.MkdirAll(filepath.Dir(*output), 0o700); err != nil {
			return err
		}
		file, err := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		if _, err = file.Write(data); err != nil {
			file.Close()
			return err
		}
		if err = file.Close(); err != nil {
			return err
		}
		fmt.Fprintf(out, "Saved encrypted vendor backup (%d bytes) to %s. No upload or restore requested.\n", len(data), *output)
		return nil
	case "prepare":
		f := flag.NewFlagSet("prepare", flag.ContinueOnError)
		f.SetOutput(out)
		action := f.String("action", "install", "install or uninstall")
		bundle := f.String("bundle", "", "current checksum-covered ARM64 script bundle")
		baseline := f.String("baseline-backup", "", "fresh authorized vendor backup downloaded in the official web UI")
		password := f.String("backup-password-file", "", "backup encryption password file")
		imei := f.String("imei-file", "", "matching device IMEI file")
		output := f.String("out", "", "new private plan directory")
		installed := f.String("installed-plan", "", "saved original install plan, required for uninstall")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if *bundle == "" || *baseline == "" || *output == "" {
			return errors.New("bundle, baseline-backup and out are required")
		}
		key, err := readKey(*password, *imei)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(*baseline)
		if err != nil {
			return err
		}
		deployment := ""
		if *action == "uninstall" {
			if *installed == "" {
				return errors.New("uninstall requires --installed-plan")
			}
			prior, _, _, err := lanbundle.Load(*installed)
			if err != nil {
				return err
			}
			if prior.Action != "install" {
				return errors.New("installed-plan must be an install plan")
			}
			deployment = prior.DeploymentID
		}
		plan, candidate, rollback, err := lanbundle.Prepare(*bundle, data, key, *action, deployment)
		if err != nil {
			return err
		}
		if err = os.MkdirAll(filepath.Dir(*output), 0o700); err != nil {
			return err
		}
		if err = lanbundle.Save(*output, plan, candidate, rollback); err != nil {
			return err
		}
		encoded, _ := json.MarshalIndent(plan, "", "  ")
		fmt.Fprintln(out, string(encoded))
		fmt.Fprintln(out, "Prepared offline. No device was contacted. Keep this private directory for uninstall/recovery.")
		if plan.Action == "install" {
			fmt.Fprintln(out, "A new candidate login token is saved in candidate-token (0600); it is not printed or extracted from the device.")
		}
		return nil
	case "run":
		f := flag.NewFlagSet("run", flag.ContinueOnError)
		f.SetOutput(out)
		dir := f.String("plan", "", "prepared plan directory")
		base := f.String("base", "http://192.168.98.1", "official device web origin")
		passwordFile := f.String("router-password-file", "", "router web password file; never passed in argv")
		backupPassword := f.String("backup-password-file", "", "backup encryption password file")
		imei := f.String("imei-file", "", "matching device IMEI file")
		work := f.String("run-dir", "", "new private execution evidence directory")
		execute := f.Bool("execute", false, "perform authenticated backup/upload/restore")
		ack := f.Bool("acknowledge-reboot", false, "acknowledge device restart and firmware restore side effects")
		timeout := f.Duration("health-timeout", 3*time.Minute, "outcome observation window per operation")
		candidateURL := f.String("candidate-health-url", "", "same-host candidate health URL, normally port 2433")
		originalURL := f.String("original-health-url", "", "same-host original health URL, normally port 2333")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if *dir == "" {
			return errors.New("plan is required")
		}
		plan, candidate, rollback, err := lanbundle.Load(*dir)
		if err != nil {
			return err
		}
		client, err := laninstall.NewClient(*base, "", nil)
		if err != nil {
			return err
		}
		u, _ := url.Parse(*base)
		if *candidateURL == "" {
			*candidateURL = u.Scheme + "://" + net.JoinHostPort(u.Hostname(), "2433") + "/healthz"
		}
		if *originalURL == "" {
			*originalURL = u.Scheme + "://" + net.JoinHostPort(u.Hostname(), "2333") + "/healthz"
		}
		options := laninstall.PlanOptions{Plan: plan, Candidate: candidate, Rollback: rollback, RunDir: *work, CandidateURL: *candidateURL, OriginalURL: *originalURL, Execute: *execute, Acknowledged: *ack, Timeout: *timeout}
		if *execute {
			if !*ack || *work == "" || *passwordFile == "" {
				return errors.New("execute requires acknowledge-reboot, run-dir and router-password-file")
			}
			options.BackupPassword, err = readKey(*backupPassword, *imei)
			if err != nil {
				return err
			}
			password, err := os.ReadFile(*passwordFile)
			if err != nil {
				return err
			}
			options.RouterPassword = strings.TrimRight(string(password), "\r\n")
			if err = os.MkdirAll(filepath.Dir(*work), 0o700); err != nil {
				return err
			}
		}
		result, runErr := client.RunPlan(ctx, options)
		encoded, _ := json.MarshalIndent(result, "", "  ")
		fmt.Fprintln(out, string(encoded))
		if !*execute {
			fmt.Fprintln(out, "Dry run: no login, backup request, upload or restart performed.")
		}
		return runErr
	default:
		return errors.New("command must be backup, prepare or run")
	}
}
