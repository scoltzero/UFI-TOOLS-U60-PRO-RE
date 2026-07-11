// SPDX-License-Identifier: AGPL-3.0-or-later

package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/config"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/device"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/store"
)

type Dependencies struct {
	Config  config.Config
	Store   *store.Store
	Device  device.Device
	Logger  *slog.Logger
	Version string
}

type handler struct {
	deps Dependencies
	auth authenticator
}

type envelope struct {
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

func New(deps Dependencies) http.Handler {
	h := &handler{deps: deps, auth: authenticator{cfg: deps.Config}}
	public := http.NewServeMux()
	public.HandleFunc("GET /healthz", h.health)
	public.HandleFunc("GET /api/version", h.version)
	public.HandleFunc("GET /api/need_token", h.needToken)

	protected := http.NewServeMux()
	protected.HandleFunc("GET /api/config", h.getConfig)
	protected.HandleFunc("POST /api/config", h.setConfig)
	protected.HandleFunc("GET /api/device/status", h.deviceStatus)
	protected.HandleFunc("GET /api/goform", h.goFormGet)
	protected.HandleFunc("POST /api/goform", h.goFormSet)
	protected.HandleFunc("GET /api/time/status", h.timeStatus)
	protected.HandleFunc("POST /api/time/sync", h.timeSync)
	protected.HandleFunc("POST /api/run_shell", h.runShell)

	root := http.NewServeMux()
	root.Handle("GET /healthz", public)
	root.Handle("GET /api/version", public)
	root.Handle("GET /api/need_token", public)
	root.Handle("/", h.auth.middleware(protected))
	return limitBody(securityHeaders(root))
}

func (h *handler) health(w http.ResponseWriter, r *http.Request) {
	if err := h.deps.Store.Ready(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, envelope{Code: 503, Message: "storage unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: map[string]bool{"ok": true}})
}

func (h *handler) version(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: map[string]string{
		"name": "UFI-TOOLS-U60-PRO-RE", "version": h.deps.Version,
	}})
}

func (h *handler) needToken(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"need_token": h.auth.required()})
}

func (h *handler) getConfig(w http.ResponseWriter, r *http.Request) {
	nickname, err := h.deps.Store.Get(r.Context(), "config", "nickname")
	if errors.Is(err, sql.ErrNoRows) {
		nickname = h.deps.Config.Nickname
	} else if err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "read configuration"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: map[string]any{
		"nickname":             nickname,
		"plugin_server_url":    h.deps.Config.PluginServerURL,
		"resource_server_url":  h.deps.Config.ResourceServerURL,
		"legacy_auth_enabled":  h.deps.Config.LegacyAuthEnabled,
		"legacy_shell_enabled": h.deps.Config.LegacyShellEnabled,
	}})
}

func (h *handler) setConfig(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Nickname string `json:"nickname"`
	}
	if err := decodeJSON(r, &request); err != nil || request.Nickname == "" {
		writeJSON(w, http.StatusBadRequest, envelope{Code: 400, Message: "nickname is required"})
		return
	}
	if err := h.deps.Store.Set(r.Context(), "config", "nickname", request.Nickname); err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "save configuration"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0})
}

func (h *handler) deviceStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.deps.Device.Status(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, envelope{Code: 502, Message: "device unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: status})
}

func (h *handler) goFormGet(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("cmd")
	if key == "" {
		key = r.URL.Query().Get("key")
	}
	value, err := h.deps.Device.GoFormGet(r.Context(), key)
	if err != nil {
		writeJSON(w, http.StatusNotFound, envelope{Code: 404, Message: "unsupported goform key"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: map[string]string{key: value}})
}

func (h *handler) goFormSet(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := decodeJSON(r, &request); err != nil || request.Key == "" {
		writeJSON(w, http.StatusBadRequest, envelope{Code: 400, Message: "key is required"})
		return
	}
	if err := h.deps.Device.GoFormSet(r.Context(), request.Key, request.Value); err != nil {
		writeJSON(w, http.StatusBadGateway, envelope{Code: 502, Message: "goform operation failed"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0})
}

func (h *handler) timeStatus(w http.ResponseWriter, _ *http.Request) {
	now := time.Now()
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: map[string]any{
		"unix": now.Unix(), "rfc3339": now.Format(time.RFC3339), "timezone": now.Location().String(),
	}})
}

func (h *handler) timeSync(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusNotImplemented, envelope{Code: 501, Message: "host time mutation is unavailable in simulator mode"})
}

func (h *handler) runShell(w http.ResponseWriter, _ *http.Request) {
	if !h.deps.Config.LegacyShellEnabled {
		writeJSON(w, http.StatusForbidden, envelope{Code: 403, Message: "legacy shell is disabled"})
		return
	}
	writeJSON(w, http.StatusNotImplemented, envelope{Code: 501, Message: "no command runner is configured"})
}

func decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("multiple JSON values")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		next.ServeHTTP(w, r)
	})
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
