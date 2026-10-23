// SPDX-License-Identifier: AGPL-3.0-or-later

package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"time"
)

func (h *handler) systemStatus(command, field string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.deps.Runner == nil {
			writeJSON(w, http.StatusServiceUnavailable, envelope{Code: 503, Message: "system runner unavailable"})
			return
		}
		result, err := h.deps.Runner.Run(r.Context(), command)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "read system state"})
			return
		}
		writeJSON(w, http.StatusOK, envelope{Code: 0, Data: map[string]any{field: strings.TrimSpace(result.Output), "exit_code": result.ExitCode}})
	}
}

func (h *handler) setWakelock(w http.ResponseWriter, r *http.Request) {
	if !h.deps.Config.PrivilegedCompat || h.deps.Config.DeviceBackend != "u60-local" {
		writeLegacyError(w, 403, "wakelock requires privileged local backend")
		return
	}
	if h.deps.Runner == nil {
		writeJSON(w, http.StatusServiceUnavailable, envelope{Code: 503, Message: "system runner unavailable"})
		return
	}
	var request struct {
		Enabled         *bool `json:"enabled"`
		WakelockEnabled *bool `json:"wakelock_enabled"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, envelope{Code: 400, Message: "invalid wakelock setting"})
		return
	}
	if request.Enabled == nil {
		request.Enabled = request.WakelockEnabled
	}
	if request.Enabled == nil {
		writeLegacyError(w, 400, "enabled is required")
		return
	}
	command := "printf ufi-tools-re > /sys/power/wake_unlock"
	if *request.Enabled {
		command = "printf ufi-tools-re > /sys/power/wake_lock"
	}
	result, err := h.deps.Runner.Run(r.Context(), command)
	if err != nil || result.ExitCode != 0 {
		writeJSON(w, http.StatusBadGateway, envelope{Code: 502, Message: "change wakelock state", Data: result})
		return
	}
	_ = h.deps.Store.Set(r.Context(), "kv", "wakelock_enabled", strings.ToLower(strings.TrimSpace(toString(*request.Enabled))))
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: map[string]bool{"enabled": *request.Enabled}})
}

func (h *handler) wifiQRCodeData(w http.ResponseWriter, r *http.Request) {
	writeLegacyError(w, 422, "saved Wi-Fi passphrases are not exposed; use the Wi-Fi form to set a new passphrase")
}

func (h *handler) speedtest(w http.ResponseWriter, r *http.Request) {
	if h.deps.Runner == nil {
		writeJSON(w, http.StatusServiceUnavailable, envelope{Code: 503, Message: "system runner unavailable"})
		return
	}
	command := shellQuote(h.deps.Config.SpeedtestBinary) + " --format=json"
	result, err := h.deps.Runner.Run(r.Context(), command)
	if err != nil || result.ExitCode != 0 {
		writeJSON(w, http.StatusBadGateway, envelope{Code: 502, Message: "speedtest failed", Data: result})
		return
	}
	var parsed any
	if json.Unmarshal([]byte(result.Output), &parsed) != nil {
		parsed = result.Output
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: parsed})
}

func (h *handler) getTask(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	item, err := h.deps.Store.GetScheduler(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "read task"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: item})
}

func (h *handler) removeTaskBody(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ID string `json:"id"`
	}
	if err := decodeJSON(r, &request); err != nil || request.ID == "" {
		writeJSON(w, http.StatusBadRequest, envelope{Code: 400, Message: "task id is required"})
		return
	}
	if err := h.deps.Store.DeleteScheduler(r.Context(), request.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "remove task"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0})
}

func (h *handler) deleteImage(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &request); err != nil || filepath.Base(request.Name) != request.Name || request.Name == "" {
		writeJSON(w, http.StatusBadRequest, envelope{Code: 400, Message: "valid image name is required"})
		return
	}
	if err := h.deps.Store.DeleteUpload(r.Context(), request.Name); err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "delete image"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0})
}

func (h *handler) deviceProxy(w http.ResponseWriter, r *http.Request) {
	if h.deps.Config.DeviceBackend != "zte-http" || h.deps.Config.DeviceBaseURL == "" {
		writeJSON(w, http.StatusConflict, envelope{Code: 409, Message: "ZTE HTTP backend is not configured"})
		return
	}
	requested := "/" + strings.TrimPrefix(r.PathValue("path"), "/")
	clean := path.Clean(requested)
	if clean != requested || !(strings.HasPrefix(clean, "/goform/") || strings.HasPrefix(clean, "/api/")) {
		writeJSON(w, http.StatusForbidden, envelope{Code: 403, Message: "proxy path is not allowed"})
		return
	}
	base, _ := url.Parse(h.deps.Config.DeviceBaseURL)
	base.Path = clean
	base.RawQuery = r.URL.RawQuery
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, base.String(), nil)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, envelope{Code: 400, Message: "invalid proxy request"})
		return
	}
	request.Header.Set("Accept", r.Header.Get("Accept"))
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return errors.New("device proxy redirects are disabled")
	}}
	response, err := client.Do(request)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, envelope{Code: 502, Message: "device proxy failed"})
		return
	}
	defer response.Body.Close()
	if contentType := response.Header.Get("Content-Type"); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, io.LimitReader(response.Body, 2<<20))
}

func firstValue(values map[string]string, keys ...string) string {
	for _, key := range keys {
		if values[key] != "" {
			return values[key]
		}
	}
	return ""
}

func escapeWiFi(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", ";", "\\;", ",", "\\,", ":", "\\:")
	return replacer.Replace(value)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
