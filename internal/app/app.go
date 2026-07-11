// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/catalog"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/config"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/device"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/httpapi"
	processrunner "github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/process"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/store"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/workers"
)

type App struct {
	cfg     config.Config
	version string
	logger  *slog.Logger
	store   *store.Store
	server  *http.Server
	workers *workers.Manager
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
	if cfg.DeviceToken == "" {
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
	var runner processrunner.Runner
	if cfg.LegacyShellEnabled {
		runner = processrunner.LocalRunner{Timeout: cfg.ShellTimeout, MaxOutput: cfg.ShellMaxOutput}
	}
	ttyd := &processrunner.Supervisor{
		Binary:  cfg.TTYDBinary,
		Args:    []string{"--port", fmt.Sprintf("%d", cfg.TTYDPort), "--writable", "/bin/sh"},
		PIDFile: filepath.Join(cfg.DataDir, "ttyd.pid"),
		LogFile: filepath.Join(cfg.DataDir, "ttyd.log"),
	}

	app.server = &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpapi.New(httpapi.Dependencies{
			Config: cfg, Store: db, Device: deviceBackend, Logger: app.logger, Version: app.version, Catalog: catalogClient, Runner: runner, TTYD: ttyd,
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
		app.workers.Start(workerContext)
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
	return a.store.Close()
}
