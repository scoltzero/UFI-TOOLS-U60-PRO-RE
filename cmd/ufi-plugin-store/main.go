// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/catalog"
)

func main() {
	root := flag.String("root", "./plugin-store", "directory containing catalog.json and plugin files")
	address := flag.String("http-addr", "127.0.0.1:2340", "HTTP listen address")
	token := flag.String("token", os.Getenv("UFI_STORE_TOKEN"), "optional X-Store-Token secret")
	flag.Parse()

	catalogData, err := catalog.Load(*root)
	if err != nil {
		slog.Error("load plugin catalog", "error", err)
		os.Exit(1)
	}
	server := &http.Server{
		Addr: *address, Handler: catalog.Handler(catalogData, *token),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second,
	}
	slog.Info("starting plugin store", "address", *address, "root", *root)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("plugin store stopped", "error", err)
		os.Exit(1)
	}
}
