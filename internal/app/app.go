// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/config"
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

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", app.health)
	mux.HandleFunc("GET /api/version", app.apiVersion)

	app.server = &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}
	return app, nil
}

func (a *App) ListenAndServe() error { return a.server.ListenAndServe() }

func (a *App) Shutdown(ctx context.Context) error { return a.server.Shutdown(ctx) }

func (a *App) Close() error { return a.store.Close() }

func (a *App) health(w http.ResponseWriter, r *http.Request) {
	if err := a.store.Ready(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "storage unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) apiVersion(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name":    "UFI-TOOLS-U60-PRO-RE",
		"version": a.version,
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; object-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}
