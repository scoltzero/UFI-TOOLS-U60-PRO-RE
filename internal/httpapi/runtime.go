// SPDX-License-Identifier: AGPL-3.0-or-later
package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/device"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func (h *handler) capabilities(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, envelope{Code: 0, Data: map[string]any{"backend": h.deps.Config.DeviceBackend, "workers": h.deps.Config.WorkersEnabled, "device_writes": h.deps.Config.PrivilegedCompat, "shell": h.deps.Config.LegacyShellEnabled, "at": h.deps.Config.ATEnabled, "local": h.deps.Config.DeviceBackend == "u60-local", "root": h.deps.Config.DeviceBackend == "u60-local" && os.Geteuid() == 0, "terminal": h.deps.TTYD != nil, "signed_updates": h.deps.Updater != nil}})
}
func (h *handler) runtimeConfig(key string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			var cfg struct {
				Enabled       bool   `json:"enabled"`
				IntervalHours int64  `json:"interval_hours"`
				Server        string `json:"server,omitempty"`
				Mode          string `json:"mode,omitempty"`
			}
			if decodeJSON(r, &cfg) != nil || cfg.IntervalHours < 0 || cfg.IntervalHours > 8760 {
				writeLegacyError(w, 400, "invalid runtime configuration")
				return
			}
			if cfg.Enabled && key == "time_config" && (cfg.Server == "" || len(cfg.Server) > 253 || strings.Trim(cfg.Server, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789.-:") != "") {
				writeLegacyError(w, 400, "an explicit NTP server is required")
				return
			}
			if cfg.Enabled && key == "auto_update_config" && h.deps.Updater == nil {
				writeLegacyError(w, 409, "configure an independent signed update source first")
				return
			}
			if cfg.Mode != "" && cfg.Mode != "check" && cfg.Mode != "download" && cfg.Mode != "install" {
				writeLegacyError(w, 400, "update mode must be check, download or install")
				return
			}
			encoded, _ := json.Marshal(cfg)
			if err := h.deps.Store.Set(r.Context(), "kv", key, string(encoded)); err != nil {
				writeLegacyError(w, 500, "save runtime configuration")
				return
			}
		}
		raw, err := h.deps.Store.Get(r.Context(), "kv", key)
		if errors.Is(err, sql.ErrNoRows) {
			raw = `{"enabled":false}`
			err = nil
		}
		if err != nil {
			writeLegacyError(w, 500, "read runtime configuration")
			return
		}
		var cfg map[string]any
		if json.Unmarshal([]byte(raw), &cfg) != nil {
			writeLegacyError(w, 500, "invalid stored configuration")
			return
		}
		// The worker uses 24 hours when the interval is absent or zero.
		if cfg["interval_hours"] == nil || cfg["interval_hours"] == float64(0) {
			cfg["interval_hours"] = 24
		}
		cfg["active"] = cfg["enabled"] == true && h.deps.Config.WorkersEnabled && (key != "time_config" || h.deps.Config.DeviceBackend == "u60-local" && h.deps.Config.PrivilegedCompat)
		if value, e := h.deps.Store.Get(r.Context(), "kv", key+"_status"); e == nil {
			var state any
			if json.Unmarshal([]byte(value), &state) == nil {
				cfg["runtime"] = state
			}
		}
		writeJSON(w, 200, envelope{Code: 0, Data: cfg})
	}
}
func (h *handler) usbStatus(w http.ResponseWriter, r *http.Request) {
	rpc, ok := h.deps.Device.(device.UBUSDevice)
	if !ok || h.deps.Config.DeviceBackend == "simulator" {
		writeJSON(w, 200, envelope{Code: 0, Data: map[string]any{"available": false}})
		return
	}
	usb, e1 := rpc.Call(r.Context(), "zwrt_bsp.usb", "list", map[string]any{})
	typec, e2 := rpc.Call(r.Context(), "zwrt_bsp.typec", "list", map[string]any{})
	if e1 != nil || e2 != nil {
		writeLegacyError(w, 502, "USB status unavailable")
		return
	}
	writeJSON(w, 200, envelope{Code: 0, Data: map[string]any{"usb": usb, "typec": typec, "available": true}})
}
func (h *handler) logs(w http.ResponseWriter, r *http.Request) {
	if h.deps.Logs == nil {
		writeLegacyError(w, 503, "candidate log manager unavailable")
		return
	}
	if r.Method == "DELETE" {
		if err := h.deps.Logs.Clear(); err != nil {
			writeLegacyError(w, 500, "clear candidate logs")
			return
		}
		writeJSON(w, 200, envelope{Code: 0})
		return
	}
	data, err := h.deps.Logs.Read()
	if err != nil {
		writeLegacyError(w, 500, "read candidate logs")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}

func (h *handler) usbMode(w http.ResponseWriter, r *http.Request) {
	if !h.deps.Config.PrivilegedCompat || h.deps.Config.DeviceBackend != "u60-local" {
		writeLegacyError(w, 403, "USB changes require privileged local backend")
		return
	}
	var req struct {
		Mode string `json:"mode"`
	}
	if decodeJSON(r, &req) != nil || req.Mode == "" || len(req.Mode) > 64 || strings.ContainsAny(req.Mode, "\x00\r\n") {
		writeLegacyError(w, 400, "explicit firmware USB mode required")
		return
	}
	rpc, ok := h.deps.Device.(device.UBUSDevice)
	if !ok {
		writeLegacyError(w, 422, "USB control unavailable")
		return
	}
	if _, err := rpc.Call(r.Context(), "zwrt_bsp.usb", "set", map[string]any{"mode": req.Mode}); err != nil {
		writeLegacyError(w, deviceErrorStatus(err), "set USB mode failed")
		return
	}
	writeJSON(w, 200, envelope{Code: 0, Message: "USB mode applied; reconnect may be required"})
}
