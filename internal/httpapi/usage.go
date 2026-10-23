// SPDX-License-Identifier: AGPL-3.0-or-later
package httpapi

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"time"
)

func (h *handler) cellularUsage(w http.ResponseWriter, r *http.Request) {
	start, end := r.URL.Query().Get("start"), r.URL.Query().Get("end")
	if start == "" {
		a, e1 := strconv.ParseInt(r.URL.Query().Get("startTime"), 10, 64)
		b, e2 := strconv.ParseInt(r.URL.Query().Get("endTime"), 10, 64)
		if e1 == nil && e2 == nil {
			start = time.UnixMilli(a).UTC().Format("2006-01-02")
			end = time.UnixMilli(b).UTC().Format("2006-01-02")
		}
	}
	a, e1 := time.Parse("2006-01-02", start)
	b, e2 := time.Parse("2006-01-02", end)
	if e1 != nil || e2 != nil || b.Before(a) || b.Sub(a) > 731*24*time.Hour {
		writeLegacyError(w, 400, "UTC date range of at most two years required")
		return
	}
	rows, err := h.deps.Store.DailyUsage(r.Context(), start, end)
	if err != nil {
		writeLegacyError(w, 500, "read traffic history")
		return
	}
	writeJSON(w, 200, map[string]any{"usage": rows, "timezone": "UTC", "coverage": "observed samples only"})
}
func (h *handler) trafficCommand(w http.ResponseWriter, r *http.Request, values map[string]string) bool {
	switch values["goformId"] {
	case "DATA_LIMIT_SETTING":
		enabled := boolString(values["data_volume_limit_switch"])
		size := values["data_volume_limit_size"]
		limit := float64(-1)
		if enabled == "1" {
			multiplier := float64(1)
			if len(size) > 2 {
				switch size[len(size)-2:] {
				case "GB":
					multiplier = 1 << 30
					size = size[:len(size)-2]
				case "MB":
					multiplier = 1 << 20
					size = size[:len(size)-2]
				}
			}
			n, err := strconv.ParseFloat(size, 64)
			if err != nil || n <= 0 || math.IsNaN(n) || math.IsInf(n, 0) {
				writeLegacyError(w, 400, "invalid data limit")
				return true
			}
			limit = n * multiplier
		}
		encoded, _ := json.Marshal(map[string]any{"data_flow_limit_enabled": enabled, "data_flow_max_limit": limit, "data_flow_check_daily_or_monthly": "monthly", "data_limit_status_forward_enabled": "0"})
		if err := h.deps.Store.Set(r.Context(), "kv", "data_limit", string(encoded)); err != nil {
			writeLegacyError(w, 500, "save limit")
			return true
		}
	case "FLOW_CALIBRATION_MANUAL":
		requested, err := strconv.ParseInt(values["data"], 10, 64)
		if err != nil || requested < 0 {
			writeLegacyError(w, 400, "invalid calibrated usage")
			return true
		}
		now := time.Now().UTC()
		start := now.Format("2006-01") + "-01"
		days, err := h.deps.Store.DailyUsage(r.Context(), start, now.Format("2006-01-02"))
		if err != nil {
			writeLegacyError(w, 500, "read traffic")
			return true
		}
		var total int64
		for _, day := range days {
			total += day.Usage
		}
		encoded, _ := json.Marshal(map[string]any{"period": start, "offset": requested - total})
		if err = h.deps.Store.Set(r.Context(), "kv", "traffic_calibration", string(encoded)); err != nil {
			writeLegacyError(w, 500, "save calibration")
			return true
		}
	default:
		return false
	}
	writeJSON(w, 200, map[string]any{"result": "success", "active": h.deps.Config.WorkersEnabled})
	return true
}
