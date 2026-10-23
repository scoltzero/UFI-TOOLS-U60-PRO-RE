// SPDX-License-Identifier: AGPL-3.0-or-later
package device

import (
	"math"
	"strconv"
	"strings"
)

// Bind the web UI's radio names to the firmware's measured fields. In SA the
// modem reports inactive LTE measurements as zero; those are not LTE readings.
func radioSnapshot(raw map[string]any, keys []string, out map[string]any) {
	aliases := map[string]string{
		"Z5g_rsrp": "nr5g_rsrp", "Nr_snr": "nr5g_snr", "nr_rsrq": "nr5g_rsrq", "nr_rssi": "nr5g_rssi",
		"Nr_bands": "nr5g_action_band", "Nr_fcn": "nr5g_action_channel", "Nr_bands_widths": "nr5g_bandwidth",
		"Nr_pci": "nr5g_pci", "Nr_cell_id": "nr5g_cell_id", "Lte_snr": "lte_snr", "Lte_bands": "wan_active_band",
	}
	mode := strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.ToUpper(textValue(raw["network_type"])))
	sa := mode == "SA" || mode == "5GSA" || mode == "NR5GSA"
	lteOnly := mode == "LTE" || mode == "4G" || mode == "LTEA"
	for _, key := range keys {
		nr := key == "Z5g_rsrp" || strings.HasPrefix(key, "Nr_") || key == "nr_rsrq" || key == "nr_rssi"
		lte := strings.HasPrefix(key, "Lte_") || key == "lte_rsrp" || key == "lte_rsrq" || key == "lte_rssi"
		if !nr && !lte {
			continue
		}
		if sa && lte || lteOnly && nr {
			out[key] = nil
			continue
		}
		value, present := raw[key]
		if alias := aliases[key]; !present && alias != "" {
			value, present = raw[alias]
		}
		if !present || value == nil || strings.TrimSpace(textValue(value)) == "" {
			out[key] = nil
			continue
		}
		if key == "Z5g_rsrp" || key == "nr_rssi" || key == "lte_rsrp" || key == "lte_rssi" {
			power, err := strconv.ParseFloat(textValue(value), 64)
			if err != nil || math.IsNaN(power) || math.IsInf(power, 0) || power >= 0 {
				out[key] = nil
				continue
			}
		}
		if key == "Nr_bands" || key == "Lte_bands" {
			band := strings.TrimSpace(textValue(value))
			band = strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(band, "n"), "N"), "b"), "B")
			value = band
		}
		out[key] = value
	}
}
