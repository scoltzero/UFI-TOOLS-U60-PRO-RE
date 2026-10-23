// SPDX-License-Identifier: AGPL-3.0-or-later

package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
)

const defaultDataLimit = `{"data_flow_limit_enabled":"0","data_flow_max_limit":-1,"data_flow_check_daily_or_monthly":"monthly","data_check_reference":"default","data_limit_status_forward_enabled":"0"}`

func (h *handler) legacyAT(w http.ResponseWriter, r *http.Request) {
	command := strings.TrimSpace(r.URL.Query().Get("command"))
	slot, err := parseSlot(r.URL.Query().Get("slot"))
	if err != nil || len(command) > 512 || !strings.HasPrefix(strings.ToUpper(command), "AT") || strings.ContainsRune(command, 0) {
		writeLegacyError(w, http.StatusBadRequest, "AT command must start with AT and use slot 0 or 1")
		return
	}
	output, err := h.runAT(r, command, slot)
	if err != nil {
		writeLegacyError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"result": normalizeATOutput(output)})
}

func (h *handler) legacyIMSStatus(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slot, err := parseSlot(r.URL.Query().Get("slot"))
		if err != nil {
			writeLegacyError(w, http.StatusBadRequest, "slot must be 0 or 1")
			return
		}
		command := "AT+CAVIMS?"
		if kind == "vonr" {
			command = `AT+SP5GCMDS="get nr synch_param",42`
		}
		output, runErr := h.runAT(r, command, slot)
		if runErr == nil {
			if enabled, parseErr := parseIMSState(kind, output); parseErr == nil {
				writeJSON(w, http.StatusOK, map[string]bool{"enabled": enabled})
				return
			}
		}
		writeLegacyError(w, 503, "IMS state unavailable")
	}
}

func (h *handler) legacySetIMSStatus(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Enabled any `json:"enabled"`
			Slot    any `json:"slot"`
		}
		if err := decodeJSON(r, &request); err != nil {
			writeLegacyError(w, http.StatusBadRequest, "invalid IMS setting")
			return
		}
		slotText := strings.Trim(toString(request.Slot), `"`)
		if slotText == "" {
			slotText = r.URL.Query().Get("slot")
		}
		slot, err := parseSlot(slotText)
		if err != nil {
			writeLegacyError(w, http.StatusBadRequest, "slot must be 0 or 1")
			return
		}
		enabled := strings.Trim(toString(request.Enabled), `"`) != "0" && strings.ToLower(strings.Trim(toString(request.Enabled), `"`)) != "false"
		state := "0"
		if enabled {
			state = "1"
		}
		command := "AT+CAVIMS=" + state
		if kind == "vonr" {
			command = `AT+SP5GCMDS="set nr param",45,` + state
		}
		output, err := h.runAT(r, command, slot)
		if err != nil || !strings.Contains(strings.ToUpper(output), "OK") {
			writeLegacyError(w, http.StatusBadGateway, "IMS command failed")
			return
		}
		if err := h.deps.Store.Set(r.Context(), "kv", fmt.Sprintf("%s_status_%d", kind, slot), state); err != nil {
			writeLegacyError(w, http.StatusInternalServerError, "save IMS state")
			return
		}
		writeLegacySuccess(w)
	}
}

func (h *handler) runAT(r *http.Request, command string, slot int) (string, error) {
	if !h.deps.Config.ATEnabled {
		return "", errors.New("AT compatibility is disabled; start with --at-enabled after installing the helper")
	}
	if h.deps.Runner == nil {
		return "", errors.New("system runner unavailable")
	}
	result, err := h.deps.Runner.Run(r.Context(), shellQuote(h.deps.Config.ATBinary)+" -n "+strconv.Itoa(slot)+" -c "+shellQuote(command))
	if err != nil || result.ExitCode != 0 {
		return result.Output, errors.New("AT helper failed")
	}
	return result.Output, nil
}

func parseSlot(value string) (int, error) {
	if value == "" {
		return 0, nil
	}
	slot, err := strconv.Atoi(value)
	if err != nil || slot < 0 || slot > 1 {
		return 0, errors.New("invalid slot")
	}
	return slot, nil
}

func normalizeATOutput(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(value, "\r", ""), "\n", " "))
	value = strings.TrimPrefix(value, ",")
	return strings.TrimSpace(value)
}

func parseIMSState(kind, output string) (bool, error) {
	body := strings.SplitN(output, ":", 2)
	if len(body) != 2 {
		return false, errors.New("unexpected IMS response")
	}
	parts := strings.Split(strings.SplitN(body[1], "OK", 2)[0], ",")
	index := 0
	if kind == "vonr" {
		index = 2
	}
	if len(parts) <= index {
		return false, errors.New("unexpected IMS response")
	}
	state, err := strconv.Atoi(strings.Trim(strings.TrimSpace(parts[index]), `"`))
	if err != nil {
		return false, err
	}
	return state == 1, nil
}

func (h *handler) legacyGetCookie(w http.ResponseWriter, r *http.Request) {
	value := ""
	if stored, err := h.deps.Store.Get(r.Context(), "kv", "official_cookie"); err == nil {
		value = strings.Trim(stored, `"`)
	}
	writeJSON(w, http.StatusOK, map[string]string{"cookie": value})
}

func (h *handler) legacySetCookie(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Cookie string `json:"cookie"`
	}
	if err := decodeJSON(r, &request); err != nil || len(request.Cookie) > 4096 || strings.ContainsAny(request.Cookie, "\r\n") {
		writeLegacyError(w, http.StatusBadRequest, "invalid cookie")
		return
	}
	encoded, _ := json.Marshal(request.Cookie)
	if err := h.deps.Store.Set(r.Context(), "kv", "official_cookie", string(encoded)); err != nil {
		writeLegacyError(w, http.StatusInternalServerError, "save cookie")
		return
	}
	writeLegacySuccess(w)
}

func (h *handler) legacyGetDataLimit(w http.ResponseWriter, r *http.Request) {
	value := defaultDataLimit
	if stored, err := h.deps.Store.Get(r.Context(), "kv", "data_limit"); err == nil {
		value = stored
	}
	var decoded map[string]any
	if json.Unmarshal([]byte(value), &decoded) != nil {
		writeLegacyError(w, http.StatusInternalServerError, "invalid saved data limit")
		return
	}
	writeJSON(w, http.StatusOK, decoded)
}

func (h *handler) legacySetDataLimit(w http.ResponseWriter, r *http.Request) {
	var request map[string]any
	if err := decodeJSON(r, &request); err != nil {
		writeLegacyError(w, http.StatusBadRequest, "invalid data limit")
		return
	}
	flowType := strings.Trim(toString(request["data_flow_check_daily_or_monthly"]), `"`)
	if flowType != "daily" {
		flowType = "monthly"
	}
	reference := strings.Trim(toString(request["data_check_reference"]), `"`)
	if reference == "android" {
		writeLegacyError(w, 422, "Android traffic reference is unavailable; select candidate statistics")
		return
	}
	reference = "default"
	limit, parseErr := strconv.ParseFloat(strings.Trim(toString(request["data_flow_max_limit"]), `"`), 64)
	if parseErr != nil || math.IsNaN(limit) || math.IsInf(limit, 0) {
		writeLegacyError(w, 400, "finite traffic limit required")
		return
	}
	if limit <= 0 && boolString(request["data_flow_limit_enabled"]) == "1" {
		writeLegacyError(w, 400, "enabled limit must be positive")
		return
	}
	if limit <= 0 {
		limit = -1
	}
	normalized := map[string]any{
		"data_flow_limit_enabled":           boolString(request["data_flow_limit_enabled"]),
		"data_flow_max_limit":               limit,
		"data_flow_check_daily_or_monthly":  flowType,
		"data_check_reference":              reference,
		"data_limit_status_forward_enabled": boolString(request["data_limit_status_forward_enabled"]),
	}
	encoded, _ := json.Marshal(normalized)
	if err := h.deps.Store.Set(r.Context(), "kv", "data_limit", string(encoded)); err != nil {
		writeLegacyError(w, http.StatusInternalServerError, "save data limit")
		return
	}
	writeLegacySuccess(w)
}

func boolString(value any) string {
	text := strings.ToLower(strings.Trim(toString(value), `"`))
	if text == "1" || text == "true" || text == "on" {
		return "1"
	}
	return "0"
}

func (h *handler) legacySpeedtest(w http.ResponseWriter, r *http.Request) {
	chunks, _ := strconv.Atoi(r.URL.Query().Get("ckSize"))
	if chunks < 1 {
		chunks = 1
	}
	if chunks > 1024 {
		chunks = 1024
	}
	const chunkSize = 1 << 20
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="random.dat"`)
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Content-Length", strconv.Itoa(chunks*chunkSize))
	if _, ok := r.URL.Query()["cors"]; ok {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}
	buffer := make([]byte, chunkSize)
	for i := range buffer {
		buffer[i] = 0x66
	}
	for i := 0; i < chunks; i++ {
		if _, err := w.Write(buffer); err != nil {
			return
		}
	}
}
