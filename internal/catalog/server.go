// SPDX-License-Identifier: AGPL-3.0-or-later

package catalog

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"os"
)

func Handler(catalog *Catalog, token string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	mux.Handle("GET /api/v1/plugins", authorize(token, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"version": 1, "plugins": catalog.List()})
	})))
	mux.Handle("GET /api/v1/plugins/{id}", authorize(token, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		item, data, err := catalog.Read(r.PathValue("id"))
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "plugin integrity failure", http.StatusConflict)
			return
		}
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("X-Plugin-ID", item.ID)
		w.Header().Set("X-Plugin-Version", item.Version)
		w.Header().Set("Digest", "sha-256="+item.SHA256)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	})))
	return securityHeaders(mux)
}

func authorize(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token != "" && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Store-Token")), []byte(token)) != 1 {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
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
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
