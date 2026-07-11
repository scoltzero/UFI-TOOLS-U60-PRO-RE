// SPDX-License-Identifier: AGPL-3.0-or-later

package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/smsforward"
)

func (h *handler) legacyShell(w http.ResponseWriter, r *http.Request) {
	if !h.deps.Config.LegacyShellEnabled || h.deps.Runner == nil {
		writeLegacyError(w, http.StatusForbidden, "shell compatibility is disabled")
		return
	}
	var request struct {
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	}
	if err := decodeJSON(r, &request); err != nil || strings.TrimSpace(request.Command) == "" || len(request.Command) > 4096 {
		writeLegacyError(w, http.StatusBadRequest, "invalid shell command")
		return
	}
	result, err := h.deps.Runner.Run(r.Context(), request.Command)
	if err != nil || result.ExitCode != 0 {
		writeLegacyError(w, http.StatusInternalServerError, strings.TrimSpace(result.Output))
		return
	}
	if r.URL.Path == "/api/user_shell" {
		writeJSON(w, http.StatusOK, map[string]any{"result": map[string]any{"done": true, "content": result.Output, "exit_code": result.ExitCode}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": result.Output})
}

func (h *handler) legacyOneClickShell(w http.ResponseWriter, r *http.Request) {
	if !h.deps.Config.PrivilegedCompat || h.deps.Runner == nil {
		writeLegacyError(w, http.StatusForbidden, "privileged compatibility is disabled")
		return
	}
	result, err := h.deps.Runner.Run(r.Context(), "id; test -w /data && echo U60_ROOT_READY")
	if err != nil || result.ExitCode != 0 || !strings.Contains(result.Output, "U60_ROOT_READY") {
		writeLegacyError(w, http.StatusBadGateway, "root shell preflight failed")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("U60 Pro already exposes an authorized root shell.\n" + result.Output))
}

func (h *handler) legacySMBPath(w http.ResponseWriter, r *http.Request) {
	if !h.deps.Config.PrivilegedCompat || h.deps.Runner == nil {
		writeLegacyError(w, http.StatusForbidden, "privileged compatibility is disabled")
		return
	}
	enabled := r.URL.Query().Get("enable") == "1"
	command := "test -f /data/samba/etc/smb.conf.ufi-tools-original && cp /data/samba/etc/smb.conf.ufi-tools-original /data/samba/etc/smb.conf || true; sync"
	if enabled {
		command = "test -f /data/samba/etc/smb.conf.ufi-tools-original || cp /data/samba/etc/smb.conf /data/samba/etc/smb.conf.ufi-tools-original; sed -i 's#^[[:space:]]*path[[:space:]]*=.*#path = /#' /data/samba/etc/smb.conf; sync"
	}
	result, err := h.deps.Runner.Run(r.Context(), command)
	if err != nil || result.ExitCode != 0 {
		writeLegacyError(w, http.StatusBadGateway, "change Samba path failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"result": "执行成功<br>Execution successful！"})
}

func (h *handler) legacyDisableFOTA(w http.ResponseWriter, r *http.Request) {
	if !h.deps.Config.PrivilegedCompat || h.deps.Runner == nil {
		writeLegacyError(w, http.StatusForbidden, "privileged compatibility is disabled")
		return
	}
	result, err := h.deps.Runner.Run(r.Context(), "touch /cache/fota.disabled; chmod 600 /cache/fota.disabled; stop ab-updater 2>/dev/null || true; sync")
	if err != nil || result.ExitCode != 0 {
		writeLegacyError(w, http.StatusBadGateway, "disable FOTA failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"result": "执行成功,如需强力禁用请使用高级功能！"})
}

func (h *handler) unsupportedAPK(w http.ResponseWriter, _ *http.Request) {
	writeLegacyError(w, http.StatusConflict, "APK installation is not applicable to the Linux U60 Pro target")
}

func (h *handler) unsupportedAPKStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "unsupported", "percent": 0, "error": "APK installation is not applicable to U60 Pro"})
}

func (h *handler) legacySMSGet(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := "sms_forward_" + kind
		value, err := h.deps.Store.Get(r.Context(), "kv", key)
		if errors.Is(err, sql.ErrNoRows) {
			value = "{}"
		} else if err != nil {
			writeLegacyError(w, http.StatusInternalServerError, "read SMS forwarding setting")
			return
		}
		var object map[string]any
		_ = json.Unmarshal([]byte(value), &object)
		if object == nil {
			object = map[string]any{}
		}
		switch kind {
		case "enabled":
			object = map[string]any{"enabled": boolString(firstObjectValue(object, "enabled", "enable", "value"))}
		case "method":
			method := firstObjectString(object, "method", "sms_forward_method")
			if method == "" {
				var direct string
				_ = json.Unmarshal([]byte(value), &direct)
				method = direct
			}
			object = map[string]any{"sms_forward_method": method}
		case "blacklist":
			object = map[string]any{"phone": firstObjectString(object, "phone"), "keywords": firstObjectString(object, "keywords")}
		case "curl":
			object = map[string]any{"curl_text": firstObjectString(object, "curl_text", "body")}
		case "dingtalk":
			object = map[string]any{"webhook_url": firstObjectString(object, "webhook_url", "webhook", "url"), "secret": firstObjectString(object, "secret"), "forward_dev_info": firstObjectString(object, "forward_dev_info")}
		case "mail":
			object = map[string]any{
				"smtp_host": firstObjectString(object, "smtp_host", "host"), "smtp_port": firstObjectString(object, "smtp_port", "port"),
				"smtp_to": firstObjectString(object, "smtp_to", "to"), "smtp_username": firstObjectString(object, "smtp_username", "username"),
				"smtp_password": firstObjectString(object, "smtp_password", "password"), "forward_dev_info": firstObjectString(object, "forward_dev_info"),
			}
		}
		writeJSON(w, http.StatusOK, object)
	}
}

func (h *handler) legacySMSSet(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body := map[string]any{}
		if r.ContentLength != 0 {
			if err := decodeJSON(r, &body); err != nil {
				writeLegacyError(w, http.StatusBadRequest, "invalid SMS forwarding setting")
				return
			}
		}
		if value := r.URL.Query().Get("enable"); value != "" {
			body["enabled"] = value
		}
		if kind == "curl" {
			body["url"] = firstObjectString(body, "url")
			body["body"] = firstObjectString(body, "body", "curl_text")
			_ = h.deps.Store.Set(r.Context(), "kv", "sms_forward_method", `"CURL"`)
		} else if kind == "dingtalk" {
			body["webhook"] = firstObjectString(body, "webhook", "webhook_url")
			_ = h.deps.Store.Set(r.Context(), "kv", "sms_forward_method", `"DINGTALK"`)
		} else if kind == "mail" {
			body["host"] = firstObjectString(body, "host", "smtp_host")
			body["to"] = firstObjectString(body, "to", "smtp_to")
			body["username"] = firstObjectString(body, "username", "smtp_username")
			body["password"] = firstObjectString(body, "password", "smtp_password")
			_ = h.deps.Store.Set(r.Context(), "kv", "sms_forward_method", `"SMTP"`)
		}
		encoded, _ := json.Marshal(body)
		if err := h.deps.Store.Set(r.Context(), "kv", "sms_forward_"+kind, string(encoded)); err != nil {
			writeLegacyError(w, http.StatusInternalServerError, "save SMS forwarding setting")
			return
		}
		writeLegacySuccess(w)
	}
}

func (h *handler) legacyPowerForwardGet(w http.ResponseWriter, r *http.Request) {
	value := "0"
	if stored, err := h.deps.Store.Get(r.Context(), "kv", "power_status_forward_enabled"); err == nil {
		var body map[string]any
		if json.Unmarshal([]byte(stored), &body) == nil {
			value = boolString(firstObjectValue(body, "enabled", "enable", "value"))
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"enabled": value})
}

func (h *handler) legacyPowerForwardSet(w http.ResponseWriter, r *http.Request) {
	value := r.URL.Query().Get("enable")
	if value == "" {
		var body map[string]any
		if err := decodeJSON(r, &body); err == nil {
			value = boolString(firstObjectValue(body, "enabled", "enable", "value"))
		}
	}
	if err := h.deps.Store.Set(r.Context(), "kv", "power_status_forward_enabled", `{"enabled":"`+boolString(value)+`"}`); err != nil {
		writeLegacyError(w, http.StatusInternalServerError, "save power forwarding setting")
		return
	}
	writeLegacySuccess(w)
}

func (h *handler) legacyDoForwardMessage(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Address   string `json:"address"`
		Body      string `json:"sms_body"`
		Timestamp int64  `json:"timestamp"`
	}
	if err := decodeJSON(r, &request); err != nil || request.Address == "" || request.Body == "" {
		writeLegacyError(w, http.StatusBadRequest, "address and sms_body are required")
		return
	}
	if request.Timestamp <= 0 {
		request.Timestamp = time.Now().UnixMilli()
	}
	manager := smsforward.New(h.deps.Store, h.deps.Device, time.Minute, nil, h.deps.Logger)
	if err := manager.Forward(r.Context(), smsforward.Message{From: request.Address, Body: request.Body, Date: time.UnixMilli(request.Timestamp).Format(time.RFC3339)}); err != nil {
		writeLegacyError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeLegacySuccess(w)
}

func firstObjectValue(object map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := object[key]; ok {
			return value
		}
	}
	return nil
}

func firstObjectString(object map[string]any, keys ...string) string {
	value := firstObjectValue(object, keys...)
	if value == nil {
		return ""
	}
	return strings.Trim(toString(value), `"`)
}
