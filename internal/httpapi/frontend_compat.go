// SPDX-License-Identifier: AGPL-3.0-or-later

package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/device"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/plugins"
)

const legacyDefaultTheme = `{"backgroundEnabled":"false","backgroundUrl":"","textColor":"rgba(255, 255, 255, 1)","textColorPer":"100","themeColor":"201","colorPer":"67","saturationPer":"100","brightPer":"21","opacityPer":"21","blurSwitch":"true","overlaySwitch":"true"}`

func (h *handler) legacyBaseDeviceInfo(w http.ResponseWriter, r *http.Request) {
	status, err := h.deps.Device.Status(r.Context())
	if err != nil {
		writeLegacyError(w, http.StatusBadGateway, "device unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"app_ver": h.deps.Version, "app_ver_code": "0", "model": status.Model,
		"battery": nil, "daily_data": int64(0), "monthly_data": int64(0),
		"internal_available_storage": nil, "internal_used_storage": nil, "internal_total_storage": nil,
		"external_total_storage": nil, "external_used_storage": nil, "external_available_storage": nil,
		"cpu_temp_list": []any{}, "cpu_temp": nil, "client_ip": r.RemoteAddr,
		"cpu_usage": nil, "mem_usage": nil, "cpuFreqInfo": map[string]any{},
		"cpuUsageInfo": map[string]any{}, "memInfo": map[string]any{},
		"current_now": nil, "voltage_now": nil, "is_reached_data_flow_limit": false,
		"signal": status.Signal, "network_type": status.NetworkType, "connected": status.Connected,
	})
}

func (h *handler) legacyConnInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"result": "success", "data": map[string]string{
		"tcp": "0", "tcp_active": "0", "tcp_other": "0", "tcp6": "0", "udp": "0", "udp6": "0", "unix": "0",
	}})
}

func (h *handler) legacyVersionInfo(w http.ResponseWriter, r *http.Request) {
	status, err := h.deps.Device.Status(r.Context())
	if err != nil {
		writeLegacyError(w, http.StatusBadGateway, "device unavailable")
		return
	}
	nickname := h.deps.Config.Nickname
	if value, getErr := h.deps.Store.Get(r.Context(), "config", "nickname"); getErr == nil && value != "" {
		nickname = value
	}
	accepted := false
	if value, getErr := h.deps.Store.Get(r.Context(), "kv", "terms_accepted"); getErr == nil {
		accepted, _ = strconv.ParseBool(strings.Trim(value, `"`))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"app_ver": h.deps.Version, "app_ver_code": "0", "model": status.Model,
		"nickname": nickname, "accept_terms": accepted,
	})
}

func (h *handler) legacyStableID(key string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		value, err := h.deps.Store.StableID(r.Context(), key)
		if err != nil {
			writeLegacyError(w, http.StatusInternalServerError, "create stable identifier")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{key: value})
	}
}

func (h *handler) legacyWeakToken(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"is_weak_token": len(h.deps.Config.DeviceToken) < 16})
}

func (h *handler) legacySetFlag(key string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h.deps.Store.Set(r.Context(), "kv", key, "true"); err != nil {
			writeLegacyError(w, http.StatusInternalServerError, "save flag")
			return
		}
		writeLegacySuccess(w)
	}
}

func (h *handler) legacyGetSetting(key, fallback string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		value, err := h.deps.Store.Get(r.Context(), "kv", key)
		if errors.Is(err, sql.ErrNoRows) {
			value = fallback
		} else if err != nil {
			writeLegacyError(w, http.StatusInternalServerError, "read setting")
			return
		}
		var decoded any
		if json.Unmarshal([]byte(value), &decoded) != nil {
			decoded = value
		}
		writeJSON(w, http.StatusOK, decoded)
	}
}

func (h *handler) legacySetSetting(key string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var value any
		if err := decodeJSON(r, &value); err != nil {
			writeLegacyError(w, http.StatusBadRequest, "invalid setting")
			return
		}
		encoded, err := json.Marshal(value)
		if err != nil || len(encoded) > 256<<10 {
			writeLegacyError(w, http.StatusBadRequest, "invalid setting")
			return
		}
		if err := h.deps.Store.Set(r.Context(), "kv", key, string(encoded)); err != nil {
			writeLegacyError(w, http.StatusInternalServerError, "save setting")
			return
		}
		writeLegacySuccess(w)
	}
}

func (h *handler) legacySetNickname(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Nickname string `json:"nickname"`
	}
	if err := decodeJSON(r, &request); err != nil || len(strings.TrimSpace(request.Nickname)) > 255 {
		writeLegacyError(w, http.StatusBadRequest, "invalid nickname")
		return
	}
	if err := h.deps.Store.Set(r.Context(), "config", "nickname", strings.TrimSpace(request.Nickname)); err != nil {
		writeLegacyError(w, http.StatusInternalServerError, "save nickname")
		return
	}
	writeLegacySuccess(w)
}

func (h *handler) legacyResourceServer(w http.ResponseWriter, r *http.Request) {
	value := h.deps.Config.ResourceServerURL
	if stored, err := h.deps.Store.Get(r.Context(), "kv", "resource_server_url"); err == nil {
		var body map[string]any
		if json.Unmarshal([]byte(stored), &body) == nil {
			value = strings.TrimSpace(toString(body["res_server"]))
		} else {
			value = strings.Trim(stored, `"`)
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"res_server": value})
}

func (h *handler) legacyLogStatus(w http.ResponseWriter, r *http.Request) {
	value := "true"
	if stored, err := h.deps.Store.Get(r.Context(), "kv", "log_enabled"); err == nil {
		var body map[string]any
		if json.Unmarshal([]byte(stored), &body) == nil {
			value = strings.TrimSpace(toString(body["debug_log_enabled"]))
		} else {
			value = strings.Trim(stored, `"`)
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"debug_log_enabled": value})
}

func (h *handler) legacyADBWiFiSetting(w http.ResponseWriter, r *http.Request) {
	enabled := false
	if value, err := h.deps.Store.Get(r.Context(), "kv", "adb_wifi_enabled"); err == nil {
		enabled, _ = strconv.ParseBool(strings.Trim(value, `"`))
	}
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": enabled})
}

func (h *handler) legacyOfficialPassword(w http.ResponseWriter, r *http.Request) {
	value := ""
	if stored, err := h.deps.Store.Get(r.Context(), "kv", "official_web_password"); err == nil {
		var body map[string]any
		if json.Unmarshal([]byte(stored), &body) == nil {
			value = strings.TrimSpace(toString(body["password"]))
		} else {
			value = strings.Trim(stored, `"`)
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"pwd": value})
}

func (h *handler) legacyGetCustomHead(w http.ResponseWriter, r *http.Request) {
	source, err := h.deps.Store.Get(r.Context(), "kv", "custom_head")
	if errors.Is(err, sql.ErrNoRows) {
		source = ""
	} else if err != nil {
		writeLegacyError(w, http.StatusInternalServerError, "read plugins")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"text": source})
}

func (h *handler) legacySetCustomHead(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Text   string `json:"text"`
		Source string `json:"source"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeLegacyError(w, http.StatusBadRequest, "invalid plugin document")
		return
	}
	source := request.Text
	if source == "" {
		source = request.Source
	}
	if source != "" {
		if _, err := plugins.Parse(source); err != nil {
			writeLegacyError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
	}
	if err := h.deps.Store.Set(r.Context(), "kv", "custom_head", source); err != nil {
		writeLegacyError(w, http.StatusInternalServerError, "save plugins")
		return
	}
	writeLegacySuccess(w)
}

func (h *handler) legacyGoFormGet(w http.ResponseWriter, r *http.Request) {
	keys := strings.Split(r.URL.Query().Get("cmd"), ",")
	filtered := keys[:0]
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key != "" {
			filtered = append(filtered, key)
		}
	}
	if len(filtered) == 0 {
		writeLegacyError(w, http.StatusBadRequest, "cmd is required")
		return
	}
	values, err := device.GetMany(r.Context(), h.deps.Device, filtered)
	if err != nil {
		writeLegacyError(w, http.StatusBadGateway, "goform request failed")
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func (h *handler) legacyGoFormSet(w http.ResponseWriter, r *http.Request) {
	values := map[string]string{}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		if err := r.ParseForm(); err != nil {
			writeLegacyError(w, http.StatusBadRequest, "invalid form")
			return
		}
		for key, items := range r.PostForm {
			if len(items) != 0 {
				values[key] = items[len(items)-1]
			}
		}
	} else {
		var body map[string]any
		if err := decodeJSON(r, &body); err != nil {
			writeLegacyError(w, http.StatusBadRequest, "invalid form")
			return
		}
		for key, value := range body {
			values[key] = strings.Trim(toString(value), `"`)
		}
	}
	result, err := device.CommandResult(r.Context(), h.deps.Device, values)
	if err != nil {
		writeLegacyError(w, http.StatusBadGateway, "goform operation failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeLegacySuccess(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]string{"result": "success"})
}

func writeLegacyError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
