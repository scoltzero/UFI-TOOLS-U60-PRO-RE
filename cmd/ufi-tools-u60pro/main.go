// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/app"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/config"
)

var version = "dev"

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(2)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel(),
	}))
	slog.SetDefault(logger)

	service, err := app.New(cfg, app.WithVersion(version), app.WithLogger(logger))
	if err != nil {
		logger.Error("initialize service", "error", err)
		os.Exit(1)
	}
	defer service.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := service.Shutdown(shutdownCtx); err != nil {
			logger.Error("graceful shutdown", "error", err)
		}
	}()

	logger.Info("starting service", "address", cfg.HTTPAddr, "data_dir", cfg.DataDir, "version", version)
	if err := service.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("service stopped", "error", err)
		os.Exit(1)
	}
}
