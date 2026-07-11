// SPDX-License-Identifier: AGPL-3.0-or-later

package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/device"
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
	mux.HandleFunc("POST /api/set_nickname", h.setNickname)
	mux.HandleFunc("POST /api/set_token", h.setToken)
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
	mux.HandleFunc("GET /api/hasTTYD", h.ttydStatus)
	mux.HandleFunc("GET /api/ttyd/status", h.ttydStatus)
	mux.HandleFunc("POST /api/ttyd/start", h.ttydStart)
	mux.HandleFunc("POST /api/ttyd/stop", h.ttydStop)
	mux.HandleFunc("GET /api/adb/status", h.adbStatus)
	mux.HandleFunc("POST /api/adb/mode", h.adbMode)
	mux.HandleFunc("GET /api/adb_alive", h.adbStatus)
	mux.HandleFunc("GET /api/adb_wifi_setting", h.getKV("adb_wifi_enabled", "false"))
	mux.HandleFunc("POST /api/adb_wifi_setting", h.adbMode)
	mux.HandleFunc("GET /api/get_official_web_password", h.getKV("official_web_password", ""))
	mux.HandleFunc("POST /api/update_admin_pwd", h.setKV("official_web_password"))
	mux.HandleFunc("GET /api/check_update", h.updateCheck)
	mux.HandleFunc("POST /api/download_update", h.updateDownload)
	mux.HandleFunc("GET /api/download_update_status", h.updateStatus)
	mux.HandleFunc("POST /api/install_update", h.updateInstall)
	mux.HandleFunc("GET /api/auto_update/check_now", h.updateCheck)
	mux.HandleFunc("GET /api/plugin/list", h.pluginList)
	mux.HandleFunc("POST /api/plugin/install", h.pluginInstallAlias)
	mux.HandleFunc("POST /api/plugin/download", h.pluginInstallAlias)
	mux.HandleFunc("POST /api/plugin/uninstall", h.pluginUninstall)
	mux.HandleFunc("GET /api/plugin/update/check", h.pluginUpdateCheck)
	mux.HandleFunc("POST /api/plugin/update/download", h.pluginInstallAlias)
	mux.HandleFunc("GET /api/plugin/notifications", h.emptyPluginEvents)
	mux.HandleFunc("POST /api/plugin/notification/read", h.emptyPluginEvents)
	mux.HandleFunc("GET /api/plugin/changelog", h.emptyPluginEvents)
	mux.HandleFunc("GET /api/SELinux", h.systemStatus("getenforce", "mode"))
	mux.HandleFunc("GET /api/usb_status", h.systemStatus("getprop sys.usb.state", "state"))
	mux.HandleFunc("POST /api/set_wakelock_status", h.setWakelock)
	mux.HandleFunc("GET /api/wifi/qrcode", h.wifiQRCodeData)
	mux.HandleFunc("POST /api/speedtest", h.speedtest)
	mux.HandleFunc("POST /api/upload_img", h.uploadFile)
	mux.HandleFunc("DELETE /api/delete_img", h.deleteImage)
	mux.HandleFunc("GET /api/get_task", h.getTask)
	mux.HandleFunc("POST /api/remove_task", h.removeTaskBody)
	mux.HandleFunc("GET /api/proxy/{path...}", h.deviceProxy)
	mux.HandleFunc("POST /api/hosts_restore", h.restoreHosts)
	mux.HandleFunc("GET /api/goform/goform_get_cmd_process", h.goFormGet)
	mux.HandleFunc("POST /api/goform/goform_set_cmd_process", h.goFormSet)
	mux.HandleFunc("GET /api/apn/all", h.apnAll)
	mux.HandleFunc("GET /api/apn/auto", h.apnAll)
	mux.HandleFunc("GET /api/apn/manual", h.apnAll)
	mux.HandleFunc("POST /api/apn/manual", h.apnSave)
	mux.HandleFunc("DELETE /api/apn/manual/{id}", h.apnDelete)
	mux.HandleFunc("GET /api/apn/mode", h.apnModeGet)
	mux.HandleFunc("POST /api/apn/mode", h.apnModeSet)
	mux.HandleFunc("GET /api/apn/cid", h.apnCID)
	mux.HandleFunc("GET /api/apn/manual/enabled", h.apnEnabled)
	mux.HandleFunc("POST /api/apn/manual/enable", h.apnEnable)

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

func (h *handler) setToken(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(r, &request); err != nil || len(request.Token) < 16 || len(request.Token) > 256 {
		writeJSON(w, http.StatusBadRequest, envelope{Code: 400, Message: "token must be between 16 and 256 characters"})
		return
	}
	if err := h.deps.Store.Set(r.Context(), "config", "device_token", request.Token); err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "save token"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Message: "token saved; restart the service to activate it"})
}

func (h *handler) setNickname(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Nickname string `json:"nickname"`
	}
	if err := decodeJSON(r, &request); err != nil || strings.TrimSpace(request.Nickname) == "" || len(request.Nickname) > 128 {
		writeJSON(w, http.StatusBadRequest, envelope{Code: 400, Message: "nickname is required"})
		return
	}
	if err := h.deps.Store.Set(r.Context(), "config", "nickname", strings.TrimSpace(request.Nickname)); err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "save nickname"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0})
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

func apnKeys() []string {
	keys := []string{
		"apn_interface_version", "apn_m_profile_name", "profile_name", "apn_wan_dial",
		"apn_select", "apn_pdp_type", "apn_pdp_select", "apn_pdp_addr", "index",
		"apn_Current_index", "apn_auto_config", "apn_ipv6_apn_auto_config", "apn_mode",
		"apn_wan_apn", "apn_ppp_auth_mode", "apn_ppp_username", "apn_ppp_passwd",
		"dns_mode", "prefer_dns_manual", "standby_dns_manual", "apn_ipv6_wan_apn",
		"apn_ipv6_pdp_type", "apn_ipv6_ppp_auth_mode", "apn_ipv6_ppp_username",
		"apn_ipv6_ppp_passwd", "ipv6_dns_mode", "ipv6_prefer_dns_manual",
		"ipv6_standby_dns_manual", "apn_num_preset",
	}
	for index := 0; index < 20; index++ {
		keys = append(keys, "APN_config"+strconv.Itoa(index), "ipv6_APN_config"+strconv.Itoa(index))
	}
	return keys
}

func (h *handler) apnAll(w http.ResponseWriter, r *http.Request) {
	values, err := device.GetMany(r.Context(), h.deps.Device, apnKeys())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, envelope{Code: 502, Message: "read APN data"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: values})
}

func (h *handler) apnModeGet(w http.ResponseWriter, r *http.Request) {
	value, err := h.deps.Device.GoFormGet(r.Context(), "apn_mode")
	if err != nil {
		writeJSON(w, http.StatusBadGateway, envelope{Code: 502, Message: "read APN mode"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: map[string]string{"mode": value}})
}

func (h *handler) apnModeSet(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Mode  string `json:"mode"`
		Index int    `json:"index"`
	}
	if err := decodeJSON(r, &request); err != nil || request.Mode != "auto" && request.Mode != "manual" || request.Index < 0 || request.Index > 19 {
		writeJSON(w, http.StatusBadRequest, envelope{Code: 400, Message: "mode must be auto/manual and index 0..19"})
		return
	}
	values := map[string]string{"goformId": "APN_PROC_EX", "apn_mode": request.Mode}
	if request.Mode == "manual" {
		values["apn_action"] = "set_default"
		values["set_default_flag"] = "1"
		values["apn_pdp_type"] = ""
		values["index"] = strconv.Itoa(request.Index)
	}
	if err := device.Command(r.Context(), h.deps.Device, values); err != nil {
		writeJSON(w, http.StatusBadGateway, envelope{Code: 502, Message: "set APN mode"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0})
}

func (h *handler) apnSave(w http.ResponseWriter, r *http.Request) {
	var request map[string]any
	if err := decodeJSON(r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, envelope{Code: 400, Message: "invalid APN profile"})
		return
	}
	values := map[string]string{"goformId": "APN_PROC_EX", "apn_mode": "manual", "apn_action": "save"}
	for key, value := range request {
		if key == "goformId" || key == "apn_action" || key == "apn_mode" || len(key) > 64 {
			continue
		}
		encoded := strings.Trim(toString(value), "\"")
		if len(encoded) <= 256 {
			values[key] = encoded
		}
	}
	if err := device.Command(r.Context(), h.deps.Device, values); err != nil {
		writeJSON(w, http.StatusBadGateway, envelope{Code: 502, Message: "save APN profile"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0})
}

func (h *handler) apnDelete(w http.ResponseWriter, r *http.Request) {
	index, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || index < 0 || index > 19 {
		writeJSON(w, http.StatusBadRequest, envelope{Code: 400, Message: "APN index must be 0..19"})
		return
	}
	err = device.Command(r.Context(), h.deps.Device, map[string]string{
		"goformId": "APN_PROC_EX", "index": strconv.Itoa(index), "apn_mode": "manual", "apn_action": "delete",
	})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, envelope{Code: 502, Message: "delete APN profile"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0})
}

func (h *handler) apnCID(w http.ResponseWriter, r *http.Request) {
	index, err := strconv.Atoi(r.URL.Query().Get("cid"))
	if err != nil || index < 0 || index > 19 {
		writeJSON(w, http.StatusBadRequest, envelope{Code: 400, Message: "cid must be 0..19"})
		return
	}
	keys := []string{"APN_config" + strconv.Itoa(index), "ipv6_APN_config" + strconv.Itoa(index)}
	values, err := device.GetMany(r.Context(), h.deps.Device, keys)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, envelope{Code: 502, Message: "read APN profile"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: values})
}

func (h *handler) apnEnabled(w http.ResponseWriter, r *http.Request) {
	values, err := device.GetMany(r.Context(), h.deps.Device, []string{"apn_mode", "apn_Current_index", "apn_select"})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, envelope{Code: 502, Message: "read active APN"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: values})
}

func (h *handler) apnEnable(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Index int `json:"index"`
	}
	if err := decodeJSON(r, &request); err != nil || request.Index < 0 || request.Index > 19 {
		writeJSON(w, http.StatusBadRequest, envelope{Code: 400, Message: "index must be 0..19"})
		return
	}
	err := device.Command(r.Context(), h.deps.Device, map[string]string{
		"goformId": "APN_PROC_EX", "apn_mode": "manual", "apn_action": "set_default",
		"set_default_flag": "1", "apn_pdp_type": "", "index": strconv.Itoa(request.Index),
	})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, envelope{Code: 502, Message: "enable APN profile"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0})
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

func (h *handler) ttydStatus(w http.ResponseWriter, _ *http.Request) {
	if h.deps.TTYD == nil {
		writeJSON(w, http.StatusOK, envelope{Code: 0, Data: map[string]bool{"installed": false, "running": false}})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: h.deps.TTYD.Status()})
}

func (h *handler) ttydStart(w http.ResponseWriter, _ *http.Request) {
	if h.deps.TTYD == nil {
		writeJSON(w, http.StatusServiceUnavailable, envelope{Code: 503, Message: "ttyd supervisor unavailable"})
		return
	}
	status, err := h.deps.TTYD.Start()
	if err != nil {
		writeJSON(w, http.StatusConflict, envelope{Code: 409, Message: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: status})
}

func (h *handler) ttydStop(w http.ResponseWriter, r *http.Request) {
	if h.deps.TTYD == nil {
		writeJSON(w, http.StatusServiceUnavailable, envelope{Code: 503, Message: "ttyd supervisor unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	status, err := h.deps.TTYD.Stop(ctx)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "stop ttyd"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: status})
}

func (h *handler) adbStatus(w http.ResponseWriter, r *http.Request) {
	if h.deps.Runner == nil {
		writeJSON(w, http.StatusServiceUnavailable, envelope{Code: 503, Message: "system runner unavailable"})
		return
	}
	result, err := h.deps.Runner.Run(r.Context(), "port=$(getprop service.adb.tcp.port 2>/dev/null); pid=$(pidof adbd 2>/dev/null); printf '%s\\n%s' \"$port\" \"$pid\"")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "read ADB state"})
		return
	}
	lines := strings.SplitN(result.Output, "\n", 2)
	port := strings.TrimSpace(lines[0])
	pid := ""
	if len(lines) == 2 {
		pid = strings.TrimSpace(lines[1])
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: map[string]any{
		"enabled": port != "" && port != "-1" && port != "0", "port": port, "running": pid != "", "pid": pid,
	}})
}

func (h *handler) adbMode(w http.ResponseWriter, r *http.Request) {
	if h.deps.Runner == nil {
		writeJSON(w, http.StatusServiceUnavailable, envelope{Code: 503, Message: "system runner unavailable"})
		return
	}
	var request struct {
		Enabled  bool   `json:"enabled"`
		Mode     string `json:"mode"`
		Port     int    `json:"port"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, envelope{Code: 400, Message: "invalid ADB mode"})
		return
	}
	enabled := request.Enabled || strings.EqualFold(request.Mode, "tcp") || strings.EqualFold(request.Mode, "network")
	port := request.Port
	if port == 0 {
		port = 5555
	}
	if port < 1024 || port > 65535 {
		writeJSON(w, http.StatusBadRequest, envelope{Code: 400, Message: "ADB port must be between 1024 and 65535"})
		return
	}
	command := "setprop service.adb.tcp.port -1; stop adbd; start adbd"
	if enabled {
		command = "setprop service.adb.tcp.port " + strconv.Itoa(port) + "; stop adbd; start adbd"
	}
	result, err := h.deps.Runner.Run(r.Context(), command)
	if err != nil || result.ExitCode != 0 {
		writeJSON(w, http.StatusBadGateway, envelope{Code: 502, Message: "change ADB mode failed", Data: result})
		return
	}
	_ = h.deps.Store.Set(r.Context(), "kv", "adb_wifi_enabled", strconv.FormatBool(enabled))
	if request.Password != "" {
		_ = h.deps.Store.Set(r.Context(), "kv", "official_web_password", request.Password)
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: map[string]any{"enabled": enabled, "port": port}})
}
