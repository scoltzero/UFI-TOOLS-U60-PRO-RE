// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/catalog"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/config"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/device"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/hostsfile"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/httpapi"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/logfile"
	processrunner "github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/process"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/smsforward"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/store"
	updateverify "github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/update"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/workers"
)

type App struct {
	cfg     config.Config
	version string
	logger  *slog.Logger
	store   *store.Store
	server  *http.Server
	workers *workers.Manager
	sms     *smsforward.Manager
	cancel  context.CancelFunc
}

type Option func(*App)

func WithVersion(version string) Option { return func(a *App) { a.version = version } }

func WithLogger(logger *slog.Logger) Option { return func(a *App) { a.logger = logger } }

func New(cfg config.Config, options ...Option) (*App, error) {
	app := &App{cfg: cfg, version: "dev", logger: slog.Default()}
	for _, option := range options {
		option(app)
	}
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	db, err := store.Open(context.Background(), filepath.Join(cfg.DataDir, "ufi-tools.db"))
	if err != nil {
		return nil, err
	}
	app.store = db
	logs := &logfile.File{Path: filepath.Join(cfg.DataDir, "candidate.log"), Enabled: func() bool {
		raw, err := db.Get(context.Background(), "kv", "log_enabled")
		if errors.Is(err, sql.ErrNoRows) {
			return true
		}
		return err == nil && !strings.Contains(raw, "false") && !strings.Contains(raw, `"0"`)
	}}
	app.logger = slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: cfg.LogLevel()}))
	if cfg.DeviceToken == "" {
		seedPath := filepath.Join(cfg.DataDir, "initial-device-token")
		seeded := false
		if info, statErr := os.Lstat(seedPath); statErr == nil {
			if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > 66 {
				db.Close()
				return nil, errors.New("installer token must be a private small regular file")
			}
			seed, readErr := os.ReadFile(seedPath)
			if readErr != nil {
				db.Close()
				return nil, readErr
			}
			value := strings.TrimRight(string(seed), "\r\n")
			if len(value) != 64 || strings.Trim(value, "0123456789abcdef") != "" {
				db.Close()
				return nil, errors.New("invalid installer token")
			}
			if err := db.SeedDeviceToken(context.Background(), value); err != nil {
				db.Close()
				return nil, err
			}
			seeded = true
		} else if !errors.Is(statErr, os.ErrNotExist) {
			db.Close()
			return nil, statErr
		}
		generated, err := db.StableID(context.Background(), "device_token")
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("generate device token: %w", err)
		}
		cfg.DeviceToken = generated
		app.cfg = cfg
		if err := os.WriteFile(filepath.Join(cfg.DataDir, "device-token"), []byte(generated+"\n"), 0o600); err != nil {
			db.Close()
			return nil, fmt.Errorf("write device token file: %w", err)
		}
		if seeded {
			if err := os.Remove(seedPath); err != nil {
				db.Close()
				return nil, err
			}
		}
	}
	deviceBackend, err := buildDevice(cfg)
	if err != nil {
		db.Close()
		return nil, err
	}
	var catalogClient *catalog.Client
	if cfg.PluginServerURL != "" {
		catalogClient, err = catalog.NewClient(cfg.PluginServerURL, cfg.PluginServerToken, &http.Client{Timeout: 10 * time.Second})
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("configure plugin catalog: %w", err)
		}
	}
	var updater *updateverify.Manager
	if cfg.UpdateManifestURL != "" {
		executable, execErr := os.Executable()
		if execErr != nil {
			db.Close()
			return nil, execErr
		}
		updater, err = updateverify.NewManager(cfg.UpdateManifestURL, cfg.UpdatePublicKey, filepath.Dir(executable), &http.Client{Timeout: 30 * time.Second})
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("configure signed updates: %w", err)
		}
	}
	var runner processrunner.Runner = processrunner.LocalRunner{Timeout: cfg.ShellTimeout, MaxOutput: cfg.ShellMaxOutput}
	ttyd := &processrunner.Supervisor{
		Binary:  cfg.TTYDBinary,
		Args:    []string{"--interface", "127.0.0.1", "--port", fmt.Sprintf("%d", cfg.TTYDPort), "--base-path", "/api/terminal", "--check-origin", "--writable", "/bin/sh"},
		PIDFile: filepath.Join(cfg.DataDir, "ttyd.pid"),
		LogFile: filepath.Join(cfg.DataDir, "ttyd.log"),
	}
	var hostsManager *hostsfile.Manager
	if cfg.HostsPath != "" {
		hostsManager = &hostsfile.Manager{Target: cfg.HostsPath, Backup: filepath.Join(cfg.DataDir, "hosts.original")}
	}

	app.server = &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpapi.New(httpapi.Dependencies{
			HealthInstance:   os.Getenv("UFI_HEALTH_INSTANCE"),
			HealthDeployment: os.Getenv("UFI_HEALTH_DEPLOYMENT"),
			Logs:             logs, Config: cfg, Store: db, Device: deviceBackend, Logger: app.logger, Version: app.version, Catalog: catalogClient, Runner: runner, TTYD: ttyd, Updater: updater, Hosts: hostsManager,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}
	if cfg.WorkersEnabled {
		workerContext, cancel := context.WithCancel(context.Background())
		app.cancel = cancel
		app.workers = workers.New(workers.Config{
			SchedulerInterval: cfg.SchedulerInterval,
			CellularInterval:  cfg.CellularInterval,
		}, db, deviceBackend, app.logger)
		app.workers.SetRuntime(workers.Runtime{
			ClearLogs: logs.Clear,
			SyncTime: func(ctx context.Context, server string) error {
				if !cfg.PrivilegedCompat || cfg.DeviceBackend != "u60-local" {
					return errors.New("time synchronization requires privileged local backend")
				}
				result, err := runner.Run(ctx, "ntpd -n -q -p "+"'"+strings.ReplaceAll(server, "'", "'\\''")+"'")
				if err != nil || result.ExitCode != 0 {
					return errors.New("NTP synchronization failed")
				}
				return nil
			},
			Update: func(ctx context.Context, mode string) error {
				if updater == nil {
					return errors.New("signed update source unavailable")
				}
				if mode == "download" || mode == "install" {
					status, err := updater.Download(ctx)
					if err != nil {
						return err
					}
					if mode == "install" && status.Version != app.version {
						if !cfg.PrivilegedCompat {
							return errors.New("automatic install requires privileged mode")
						}
						return updater.Install(ctx)
					}
					return nil
				}
				_, err := updater.Check(ctx)
				return err
			},
		})
		app.workers.Start(workerContext)
		app.sms = smsforward.New(db, deviceBackend, cfg.SMSPollInterval, nil, app.logger)
		app.sms.Start(workerContext)
	}
	return app, nil
}

func buildDevice(cfg config.Config) (device.Device, error) {
	switch cfg.DeviceBackend {
	case "simulator":
		return device.NewSimulator(), nil
	case "zte-http":
		client := &http.Client{Timeout: cfg.DeviceHTTPTimeout}
		return device.NewZTEHTTP(cfg.DeviceBaseURL, client)
	case "u60-ubus":
		client := &http.Client{Timeout: cfg.DeviceHTTPTimeout}
		backend, err := device.NewU60UBUS(cfg.DeviceBaseURL, cfg.DeviceSession, client)
		if err == nil {
			backend.EnableWrites(cfg.PrivilegedCompat)
		}
		return backend, err
	case "u60-local":
		return device.NewU60Local(cfg.PrivilegedCompat), nil
	default:
		return nil, fmt.Errorf("unsupported device backend %q", cfg.DeviceBackend)
	}
}

func (a *App) ListenAndServe() error { return a.server.ListenAndServe() }

func (a *App) Shutdown(ctx context.Context) error { return a.server.Shutdown(ctx) }

func (a *App) Close() error {
	if a.cancel != nil {
		a.cancel()
	}
	if a.workers != nil {
		a.workers.Stop()
	}
	if a.sms != nil {
		a.sms.Stop()
	}
	return a.store.Close()
}
