// SPDX-License-Identifier: AGPL-3.0-or-later
package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/device"
)

func scalarString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	case json.Number:
		return x.String()
	case nil:
		return ""
	default:
		return ""
	}
}
func apnID(v string) bool { return v != "" && len(v) <= 64 && !strings.ContainsAny(v, "\r\n\x00") }
func apnPublic(v map[string]any) map[string]any {
	return map[string]any{"profileId": scalarString(v["profileId"]), "profile_name": scalarString(v["profilename"]), "apn_wan_apn": scalarString(v["wanapn"]), "apn_pdp_type": scalarString(v["pdpType"]), "apn_ppp_auth_mode": scalarString(v["pppAuthMode"])}
}
func apnProfiles(value map[string]any) ([]map[string]any, error) {
	result := []map[string]any{}
	raw := value["apnListArray"]
	if raw == nil {
		raw = value["apn_list"]
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("unrecognized APN list")
	}
	for _, row := range list {
		v, ok := row.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("unrecognized APN profile")
		}
		result = append(result, apnPublic(v))
	}
	return result, nil
}

func (h *handler) apnAll(w http.ResponseWriter, r *http.Request) {
	rpc, ok := h.deps.Device.(device.UBUSDevice)
	if !ok {
		values, err := device.GetMany(r.Context(), h.deps.Device, []string{"apn_mode", "apn_index", "wan_apn", "apn_pdp_type", "ppp_auth_mode"})
		if err != nil {
			writeLegacyError(w, 502, "read APN state")
			return
		}
		writeJSON(w, 200, envelope{Code: 0, Data: values})
		return
	}
	auto, e1 := rpc.Call(r.Context(), "zwrt_apn_object", "get_auto_apn_list", map[string]any{"slotId": 1})
	manual, e2 := rpc.Call(r.Context(), "zwrt_apn_object", "get_manu_apn_list", map[string]any{})
	mode, e3 := rpc.Call(r.Context(), "zwrt_apn_object", "get_apn_mode", map[string]any{})
	enabled, e4 := rpc.Call(r.Context(), "zwrt_apn_object", "get_enabled_manu_apn_id", map[string]any{})
	for _, err := range []error{e1, e2, e3, e4} {
		if err != nil {
			writeJSON(w, deviceErrorStatus(err), envelope{Code: deviceErrorStatus(err), Message: "read APN state failed"})
			return
		}
	}
	autoList, ae := apnProfiles(auto)
	manualList, me := apnProfiles(manual)
	if ae != nil || me != nil {
		writeJSON(w, 502, envelope{Code: 502, Message: "unrecognized APN response"})
		return
	}
	selected := "auto"
	if scalarString(mode["apn_mode"]) == "1" {
		selected = "manual"
	}
	writeJSON(w, 200, envelope{Code: 0, Data: map[string]any{"mode": selected, "auto": autoList, "manual": manualList, "enabled_id": scalarString(enabled["profileId"])}})
}
func (h *handler) apnModeGet(w http.ResponseWriter, r *http.Request) {
	rpc, ok := h.deps.Device.(device.UBUSDevice)
	if !ok {
		mode, err := h.deps.Device.GoFormGet(r.Context(), "apn_mode")
		if err != nil {
			writeLegacyError(w, 502, "read APN mode")
			return
		}
		writeJSON(w, 200, envelope{Code: 0, Data: map[string]string{"mode": mode}})
		return
	}
	data, err := rpc.Call(r.Context(), "zwrt_apn_object", "get_apn_mode", map[string]any{})
	if err != nil {
		writeJSON(w, deviceErrorStatus(err), envelope{Code: deviceErrorStatus(err), Message: "read APN mode failed"})
		return
	}
	mode := "auto"
	if scalarString(data["apn_mode"]) == "1" {
		mode = "manual"
	}
	writeJSON(w, 200, envelope{Code: 0, Data: map[string]string{"mode": mode}})
}
func (h *handler) apnModeSet(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Mode      string `json:"mode"`
		Index     *int   `json:"index"`
		ProfileID string `json:"profileId"`
	}
	if decodeJSON(r, &request) != nil || (request.Mode != "auto" && request.Mode != "manual") {
		writeJSON(w, 400, envelope{Code: 400, Message: "mode must be auto or manual"})
		return
	}
	id := request.ProfileID
	if id == "" && request.Index != nil {
		id = strconv.Itoa(*request.Index)
	}
	if request.Mode == "manual" && !apnID(id) {
		writeJSON(w, 400, envelope{Code: 400, Message: "manual profile identifier is required"})
		return
	}
	rpc, ok := h.deps.Device.(device.UBUSDevice)
	if !ok {
		values := map[string]string{"goformId": "APN_PROC_EX", "apn_mode": request.Mode, "index": id, "apn_action": "set_default", "set_default_flag": "1"}
		if err := device.Command(r.Context(), h.deps.Device, values); err != nil {
			writeLegacyError(w, deviceErrorStatus(err), "set APN mode failed")
			return
		}
		writeJSON(w, 200, envelope{Code: 0})
		return
	}
	if err := device.SelectAPN(r.Context(), rpc, request.Mode, id); err != nil {
		writeJSON(w, deviceErrorStatus(err), envelope{Code: deviceErrorStatus(err), Message: err.Error()})
		return
	}
	writeJSON(w, 200, envelope{Code: 0})
}
func (h *handler) apnSave(w http.ResponseWriter, r *http.Request) {
	var request map[string]any
	if decodeJSON(r, &request) != nil {
		writeJSON(w, 400, envelope{Code: 400, Message: "invalid APN profile"})
		return
	}
	name := strings.TrimSpace(scalarString(request["profile_name"]))
	apn := strings.TrimSpace(scalarString(request["apn_wan_apn"]))
	pdp, e1 := strconv.Atoi(scalarString(request["apn_pdp_type"]))
	auth, e2 := strconv.Atoi(scalarString(request["apn_ppp_auth_mode"]))
	id := r.PathValue("id")
	if id == "" {
		id = scalarString(request["profileId"])
	}
	if name == "" || len(name) > 64 || apn == "" || len(apn) > 100 || strings.ContainsAny(name+apn, "\r\n\x00") || e1 != nil || pdp < 1 || pdp > 3 || e2 != nil || auth < 0 || auth > 3 || id != "" && !apnID(id) {
		writeJSON(w, 400, envelope{Code: 400, Message: "profile name, APN, PDP type (1–3) and authentication mode (0–3) are required"})
		return
	}
	username, password := scalarString(request["apn_ppp_username"]), scalarString(request["apn_ppp_passwd"])
	if len(username) > 128 || len(password) > 128 || auth > 0 && (username == "" || password == "") {
		writeJSON(w, 400, envelope{Code: 400, Message: "authenticated APNs require a newly entered username and password"})
		return
	}
	rpc, ok := h.deps.Device.(device.UBUSDevice)
	if !ok {
		values := map[string]string{"goformId": "APN_PROC_EX", "apn_action": "save", "index": id}
		for k, v := range request {
			values[k] = scalarString(v)
		}
		if err := device.Command(r.Context(), h.deps.Device, values); err != nil {
			writeLegacyError(w, deviceErrorStatus(err), "save APN failed")
			return
		}
		writeJSON(w, 200, envelope{Code: 0})
		return
	}
	if password != "" {
		local, ok := h.deps.Device.(interface{ LocalControl() bool })
		if !ok || !local.LocalControl() {
			writeJSON(w, 422, envelope{Code: 422, Message: "credential-bearing APN configuration requires the local device backend"})
			return
		}
	}
	params := map[string]any{"profilename": name, "wanapn": apn, "pdpType": pdp, "pppAuthMode": auth, "username": username, "password": password}
	method := "add_manu_apn"
	if id != "" {
		method = "modify_manu_apn"
		params["profileId"] = id
	}
	if _, err := rpc.Call(r.Context(), "zwrt_apn_object", method, params); err != nil {
		writeJSON(w, deviceErrorStatus(err), envelope{Code: deviceErrorStatus(err), Message: "save APN profile failed"})
		return
	}
	writeJSON(w, 200, envelope{Code: 0})
}
func (h *handler) apnDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		id = r.URL.Query().Get("id")
	}
	if !apnID(id) {
		writeJSON(w, 400, envelope{Code: 400, Message: "profile identifier required"})
		return
	}
	rpc, ok := h.deps.Device.(device.UBUSDevice)
	if !ok {
		if err := device.Command(r.Context(), h.deps.Device, map[string]string{"goformId": "APN_PROC_EX", "apn_action": "delete", "index": id}); err != nil {
			writeLegacyError(w, deviceErrorStatus(err), "delete APN failed")
			return
		}
		writeJSON(w, 200, envelope{Code: 0})
		return
	}
	if _, err := rpc.Call(r.Context(), "zwrt_apn_object", "delete_manu_apn", map[string]any{"profileId": id}); err != nil {
		writeJSON(w, deviceErrorStatus(err), envelope{Code: deviceErrorStatus(err), Message: "delete APN profile failed"})
		return
	}
	writeJSON(w, 200, envelope{Code: 0})
}
func (h *handler) apnCID(w http.ResponseWriter, r *http.Request) {
	cid, err := strconv.Atoi(r.URL.Query().Get("cid"))
	if err != nil || cid < 1 || cid > 16 {
		writeJSON(w, 400, envelope{Code: 400, Message: "cid must be 1–16"})
		return
	}
	rpc, ok := h.deps.Device.(device.UBUSDevice)
	if !ok {
		writeJSON(w, 422, envelope{Code: 422, Message: "APN requires the U60 backend"})
		return
	}
	data, err := rpc.Call(r.Context(), "zwrt_apn_object", "get_candidate_apn_by_cid", map[string]any{"cid": cid})
	if err != nil {
		writeJSON(w, deviceErrorStatus(err), envelope{Code: deviceErrorStatus(err), Message: "read APN CID failed"})
		return
	}
	data = apnPublic(data)
	writeJSON(w, 200, envelope{Code: 0, Data: data})
}
func (h *handler) apnEnabled(w http.ResponseWriter, r *http.Request) {
	rpc, ok := h.deps.Device.(device.UBUSDevice)
	if !ok {
		writeJSON(w, 422, envelope{Code: 422, Message: "APN requires the U60 backend"})
		return
	}
	data, err := rpc.Call(r.Context(), "zwrt_apn_object", "get_enabled_manu_apn_id", map[string]any{})
	if err != nil {
		writeJSON(w, deviceErrorStatus(err), envelope{Code: deviceErrorStatus(err), Message: "read selected APN failed"})
		return
	}
	writeJSON(w, 200, envelope{Code: 0, Data: map[string]string{"profileId": scalarString(data["profileId"])}})
}
func (h *handler) apnEnable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProfileID string `json:"profileId"`
		Index     *int   `json:"index"`
	}
	if decodeJSON(r, &req) != nil {
		writeJSON(w, 400, envelope{Code: 400, Message: "invalid selection"})
		return
	}
	id := req.ProfileID
	if id == "" && req.Index != nil {
		id = strconv.Itoa(*req.Index)
	}
	if !apnID(id) {
		writeJSON(w, 400, envelope{Code: 400, Message: "profile identifier required"})
		return
	}
	rpc, ok := h.deps.Device.(device.UBUSDevice)
	if !ok {
		writeJSON(w, 422, envelope{Code: 422, Message: "APN selection requires UBUS"})
		return
	}
	if err := device.SelectAPN(r.Context(), rpc, "manual", id); err != nil {
		writeJSON(w, deviceErrorStatus(err), envelope{Code: deviceErrorStatus(err), Message: err.Error()})
		return
	}
	writeJSON(w, 200, envelope{Code: 0})
}
