// SPDX-License-Identifier: AGPL-3.0-or-later

package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

func (h *handler) registerCompatibilityRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/baseDeviceInfo", h.deviceStatus)
	mux.HandleFunc("GET /api/connInfo", h.deviceStatus)
	mux.HandleFunc("GET /api/version_info", h.version)
	mux.HandleFunc("GET /api/device_uuid", h.stableID("device_uuid"))
	mux.HandleFunc("GET /api/device_id", h.stableID("device_id"))
	mux.HandleFunc("GET /api/is_weak_token", h.weakToken)
	mux.HandleFunc("POST /api/accept_terms", h.setFlag("terms_accepted"))
	mux.HandleFunc("GET /api/get_theme", h.getKV("theme", "auto"))
	mux.HandleFunc("POST /api/set_theme", h.setKV("theme"))
	mux.HandleFunc("POST /api/set_nickname", h.setKV("nickname"))
	mux.HandleFunc("GET /api/get_res_server", h.getKV("resource_server_url", h.deps.Config.ResourceServerURL))
	mux.HandleFunc("POST /api/set_res_server", h.setKV("resource_server_url"))
	mux.HandleFunc("GET /api/get_log_status", h.getKV("log_enabled", "true"))
	mux.HandleFunc("POST /api/set_log_status", h.setKV("log_enabled"))
	mux.HandleFunc("GET /api/log_clear", h.getKV("log_clear", `{"enabled":false,"interval_hours":24}`))
	mux.HandleFunc("POST /api/log_clear", h.setKV("log_clear"))
	mux.HandleFunc("GET /api/get_custom_head", h.getCustomHead)
	mux.HandleFunc("POST /api/set_custom_head", h.setCustomHead)
	mux.HandleFunc("GET /api/plugins_store", h.localCatalog)
	mux.HandleFunc("GET /api/list_tasks", h.listSchedulers)
	mux.HandleFunc("POST /api/add_task", h.setScheduler)
	mux.HandleFunc("DELETE /api/remove_task/{id}", h.deleteScheduler)
	mux.HandleFunc("POST /api/clear_task", h.clearTasks)
	mux.HandleFunc("GET /api/list_uploads", h.listUploads)
	mux.HandleFunc("POST /api/delete_all_uploads_data", h.deleteAllUploads)
	mux.HandleFunc("GET /api/cellularUsage", h.recentCellular)
	mux.HandleFunc("POST /api/hosts_restore", h.restoreHosts)
	mux.HandleFunc("GET /api/goform/goform_get_cmd_process", h.goFormGet)
	mux.HandleFunc("POST /api/goform/goform_set_cmd_process", h.goFormSet)

	for _, route := range []struct{ path, key, fallback string }{
		{"/api/sms_forward_enabled", "sms_forward_enabled", "false"},
		{"/api/sms_forward_method", "sms_forward_method", "sms"},
		{"/api/sms_forward_blacklist", "sms_forward_blacklist", "[]"},
		{"/api/sms_forward_curl", "sms_forward_curl", "{}"},
		{"/api/sms_forward_dingtalk", "sms_forward_dingtalk", "{}"},
		{"/api/sms_forward_mail", "sms_forward_mail", "{}"},
		{"/api/power_status_forward_enabled", "power_status_forward_enabled", "false"},
		{"/api/time/config", "time_config", `{"enabled":false}`},
		{"/api/auto_update/config", "auto_update_config", `{"enabled":false}`},
	} {
		mux.HandleFunc("GET "+route.path, h.getKV(route.key, route.fallback))
		mux.HandleFunc("POST "+route.path, h.setKV(route.key))
	}

	for _, pattern := range []string{
		"GET /api/SELinux", "GET /api/usb_status", "GET /api/hasTTYD",
		"GET /api/ttyd/status", "POST /api/ttyd/start", "POST /api/ttyd/stop",
		"GET /api/adb/status", "POST /api/adb/mode", "GET /api/adb_alive",
		"POST /api/set_wakelock_status", "POST /api/update_admin_pwd",
		"GET /api/wifi/qrcode", "POST /api/speedtest",
		"GET /api/check_update", "POST /api/download_update", "GET /api/download_update_status",
		"POST /api/install_update", "GET /api/auto_update/check_now",
		"POST /api/upload_img", "DELETE /api/delete_img",
		"GET /api/plugin/list", "GET /api/plugin/changelog", "GET /api/plugin/notifications",
		"POST /api/plugin/notification/read", "GET /api/plugin/update/check",
		"POST /api/plugin/update/download", "POST /api/plugin/download", "POST /api/plugin/install",
		"POST /api/plugin/uninstall",
		"POST /api/set_token", "GET /api/get_task", "POST /api/remove_task",
		"GET /api/proxy/{path...}",
	} {
		mux.HandleFunc(pattern, h.notAvailable)
	}

	for _, route := range []struct{ pattern, key string }{
		{"GET /api/apn/mode", "apn_mode"}, {"POST /api/apn/mode", "apn_mode"},
		{"GET /api/apn/cid", "pdp_type"}, {"GET /api/apn/manual/enabled", "apn_select"},
		{"POST /api/apn/manual/enable", "apn_select"},
	} {
		mux.HandleFunc(route.pattern, h.goFormCompatibility(route.key))
	}
	for _, pattern := range []string{
		"GET /api/apn/all", "GET /api/apn/auto", "GET /api/apn/manual",
		"POST /api/apn/manual", "DELETE /api/apn/manual/{id}",
	} {
		mux.HandleFunc(pattern, h.notAvailable)
	}
}

func (h *handler) stableID(key string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		value, err := h.deps.Store.StableID(r.Context(), key)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "create stable identifier"})
			return
		}
		writeJSON(w, http.StatusOK, envelope{Code: 0, Data: map[string]string{key: value}})
	}
}

func (h *handler) weakToken(w http.ResponseWriter, _ *http.Request) {
	weak := len(h.deps.Config.DeviceToken) < 16
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: map[string]bool{"weak": weak}})
}

func (h *handler) getKV(key, fallback string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		value, err := h.deps.Store.Get(r.Context(), "kv", key)
		if errors.Is(err, sql.ErrNoRows) {
			value = fallback
		} else if err != nil {
			writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "read setting"})
			return
		}
		var decoded any
		if json.Unmarshal([]byte(value), &decoded) != nil {
			decoded = value
		}
		writeJSON(w, http.StatusOK, envelope{Code: 0, Data: decoded})
	}
}

func (h *handler) setKV(key string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var value any
		if err := decodeJSON(r, &value); err != nil {
			writeJSON(w, http.StatusBadRequest, envelope{Code: 400, Message: "invalid setting"})
			return
		}
		encoded, err := json.Marshal(value)
		if err != nil || len(encoded) > 256<<10 {
			writeJSON(w, http.StatusBadRequest, envelope{Code: 400, Message: "invalid setting size"})
			return
		}
		if err := h.deps.Store.Set(r.Context(), "kv", key, string(encoded)); err != nil {
			writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "save setting"})
			return
		}
		writeJSON(w, http.StatusOK, envelope{Code: 0})
	}
}

func (h *handler) setFlag(key string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h.deps.Store.Set(r.Context(), "kv", key, "true"); err != nil {
			writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "save flag"})
			return
		}
		writeJSON(w, http.StatusOK, envelope{Code: 0})
	}
}

func (h *handler) clearTasks(w http.ResponseWriter, r *http.Request) {
	items, err := h.deps.Store.ListSchedulers(r.Context())
	if err == nil {
		for _, item := range items {
			err = h.deps.Store.DeleteScheduler(r.Context(), item.ID)
			if err != nil {
				break
			}
		}
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "clear tasks"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0})
}

func (h *handler) deleteAllUploads(w http.ResponseWriter, r *http.Request) {
	if err := h.deps.Store.DeleteAllUploads(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "delete uploads"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0})
}

func (h *handler) goFormCompatibility(key string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			value, err := h.deps.Device.GoFormGet(r.Context(), key)
			if err != nil {
				writeJSON(w, http.StatusBadGateway, envelope{Code: 502, Message: "device operation failed"})
				return
			}
			writeJSON(w, http.StatusOK, envelope{Code: 0, Data: map[string]string{key: value}})
			return
		}
		var body map[string]any
		if err := decodeJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, envelope{Code: 400, Message: "invalid device setting"})
			return
		}
		value := strings.TrimSpace(toString(body["value"]))
		if value == "" {
			value = strings.TrimSpace(toString(body[key]))
		}
		if value == "" || h.deps.Device.GoFormSet(r.Context(), key, value) != nil {
			writeJSON(w, http.StatusBadGateway, envelope{Code: 502, Message: "device operation failed"})
			return
		}
		writeJSON(w, http.StatusOK, envelope{Code: 0})
	}
}

func toString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		encoded, _ := json.Marshal(value)
		return string(encoded)
	}
}

func (h *handler) notAvailable(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusNotImplemented, envelope{Code: 501, Message: "recognized compatibility route; privileged device adapter is not configured"})
}
