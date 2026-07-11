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

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/config"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/device"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/httpapi"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/store"
)

type App struct {
	cfg     config.Config
	version string
	logger  *slog.Logger
	store   *store.Store
	server  *http.Server
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
	deviceBackend, err := buildDevice(cfg)
	if err != nil {
		db.Close()
		return nil, err
	}

	app.server = &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpapi.New(httpapi.Dependencies{
			Config: cfg, Store: db, Device: deviceBackend, Logger: app.logger, Version: app.version,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    32 << 10,
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

func (a *App) Close() error { return a.store.Close() }
