// SPDX-License-Identifier: AGPL-3.0-or-later
package device

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestU60RadioSnapshot(t *testing.T) {
	keys := []string{"network_type", "network_signalbar", "Z5g_rsrp", "Nr_snr", "nr_rsrq", "nr_rssi", "Nr_bands", "Nr_fcn", "Nr_bands_widths", "Nr_pci", "Nr_cell_id", "lte_rsrp", "lte_rsrq", "lte_rssi", "Lte_snr", "Lte_bands", "lte_band_lock"}
	cases := []struct {
		name      string
		raw, want map[string]any
	}{
		{"SA native names and inactive LTE", map[string]any{"network_type": "SA", "signalbar": "5", "rssi": 0, "nr5g_rsrp": -66, "nr5g_snr": "24.2", "nr5g_rsrq": -11, "nr5g_rssi": -53, "nr5g_action_band": "n41", "nr5g_action_channel": 504990, "nr5g_bandwidth": "100", "nr5g_pci": 42, "nr5g_cell_id": 123456789, "wan_active_band": "n41", "lte_rsrp": 0, "lte_rsrq": 0, "lte_rssi": 0, "lte_snr": "", "lte_band_lock": "0x5"}, map[string]any{"network_signalbar": "5", "Z5g_rsrp": float64(-66), "Nr_snr": "24.2", "nr_rsrq": float64(-11), "nr_rssi": float64(-53), "Nr_bands": "41", "Nr_fcn": float64(504990), "Nr_bands_widths": "100", "Nr_pci": float64(42), "Nr_cell_id": float64(123456789), "lte_rsrp": nil, "lte_rsrq": nil, "Lte_bands": nil, "lte_band_lock": "1,3"}},
		{"NSA retains both active carriers and legitimate zeros", map[string]any{"network_type": "LTE NSA", "nr5g_rsrp": -90, "nr5g_snr": "0", "nr5g_pci": 0, "lte_rsrp": -95, "lte_snr": "0", "wan_active_band": "B3"}, map[string]any{"Z5g_rsrp": float64(-90), "Nr_pci": float64(0), "Nr_snr": "0", "lte_rsrp": float64(-95), "Lte_snr": "0", "Lte_bands": "3"}},
		{"LTE suppresses stale NR", map[string]any{"network_type": "LTE", "nr5g_rsrp": -80, "nr5g_action_band": "n41", "lte_rsrp": -92, "wan_active_band": "b3"}, map[string]any{"Z5g_rsrp": nil, "Nr_bands": nil, "lte_rsrp": float64(-92), "Lte_bands": "3"}},
		{"missing radio is unavailable", map[string]any{"network_type": "SA"}, map[string]any{"Z5g_rsrp": nil, "Nr_snr": nil, "Nr_pci": nil}},
		{"zero power is a placeholder but zero SINR and PCI are valid", map[string]any{"network_type": "NR5G-SA", "nr5g_rsrp": 0, "nr5g_rssi": 0, "nr5g_snr": "0", "nr5g_pci": 0}, map[string]any{"Z5g_rsrp": nil, "nr_rssi": nil, "Nr_snr": "0", "Nr_pci": float64(0)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var calls []struct {
					Params []any `json:"params"`
				}
				if err := json.NewDecoder(r.Body).Decode(&calls); err != nil {
					t.Error(err)
					return
				}
				method := calls[0].Params[2].(string)
				if !readOnlyUBUS(method) {
					t.Errorf("unexpected write %s", method)
				}
				values := map[string]any{}
				if method == "nwinfo_get_netinfo" {
					values = tc.raw
				}
				json.NewEncoder(w).Encode([]any{map[string]any{"result": []any{0, values}}})
			}))
			defer server.Close()
			backend, err := NewU60UBUS(server.URL, "", server.Client())
			if err != nil {
				t.Fatal(err)
			}
			result, err := backend.Snapshot(t.Context(), keys)
			if err != nil {
				t.Fatal(err)
			}
			for key, want := range tc.want {
				if result[key] != want {
					t.Errorf("%s = %#v, want %#v", key, result[key], want)
				}
			}
		})
	}
}
