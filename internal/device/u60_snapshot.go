// SPDX-License-Identifier: AGPL-3.0-or-later
package device

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"
)

func (u *U60UBUS) rawStatus(ctx context.Context) (map[string]any, error) {
	u.cacheMu.Lock()
	defer u.cacheMu.Unlock()
	if u.cache != nil && time.Since(u.cacheAt) < 500*time.Millisecond {
		return u.cache, nil
	}
	merged := map[string]any{}
	var failures []error
	for _, c := range [][2]string{{"zwrt_mc.device.manager", "get_device_info"}, {"zte_nwinfo_api", "nwinfo_get_netinfo"}, {"zwrt_zte_mdm.api", "get_sim_info_before"}, {"zwrt_router.api", "router_get_status_no_auth"}, {"zwrt_wlan", "report"}} {
		data, err := u.call(ctx, c[0], c[1], map[string]any{})
		if err != nil {
			failures = append(failures, err)
			continue
		}
		for k, v := range data {
			merged[k] = v
		}
	}
	if len(merged) == 0 {
		if len(failures) > 0 {
			return nil, errors.Join(failures...)
		}
		return nil, errors.New("device returned no status data")
	}
	u.cache, u.cacheAt = merged, time.Now()
	return merged, nil
}

func (u *U60UBUS) invalidateStatus() { u.cacheMu.Lock(); u.cache = nil; u.cacheMu.Unlock() }
func textValue(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
func firstValue(v map[string]any, keys ...string) any {
	for _, k := range keys {
		if x, ok := v[k]; ok && x != nil {
			return x
		}
	}
	return nil
}
func intValue(v any) int { n, _ := strconv.Atoi(textValue(v)); return n }

func (u *U60UBUS) section(ctx context.Context, config, section string) (map[string]any, error) {
	data, err := u.call(ctx, "uci", "get", map[string]any{"config": config, "section": section})
	if err != nil {
		return nil, err
	}
	if values, ok := data["values"].(map[string]any); ok {
		return values, nil
	}
	return data, nil
}

func (u *U60UBUS) Snapshot(ctx context.Context, keys []string) (map[string]any, error) {
	raw, err := u.rawStatus(ctx)
	if err != nil {
		return nil, err
	}
	out := u.systemSnapshot(keys)
	wanted := map[string]bool{}
	aliases := map[string]string{"imei": "modem_imei", "hw_version": "wa_version", "signal": "signalbar", "network_signalbar": "signalbar", "cr_version": "wa_inner_version", "SSID1": "main2g_ssid", "wifi_ssid": "main2g_ssid", "AuthMode": "main2g_authmode", "network_provider": "network_provider_fullname", "sim_slot": "current_sim_slot"}
	for _, key := range keys {
		wanted[key] = true
		value, present := raw[key]
		if !present && aliases[key] != "" {
			value, present = raw[aliases[key]]
		}
		if present {
			out[key] = value
		}
	}
	radioSnapshot(raw, keys, out)
	has := func(names ...string) bool {
		for _, n := range names {
			if wanted[n] {
				return true
			}
		}
		return false
	}
	if wanted["lte_band_lock"] {
		if value := textValue(raw["lte_band_lock"]); strings.HasPrefix(value, "0x") {
			mask, ok := new(big.Int).SetString(value[2:], 16)
			if ok {
				bands := []string{}
				for bit := 0; bit < mask.BitLen(); bit++ {
					if mask.Bit(bit) == 1 {
						bands = append(bands, strconv.Itoa(bit+1))
					}
				}
				out["lte_band_lock"] = strings.Join(bands, ",")
			}
		}
	}
	if wanted["nr_band_lock"] {
		out["nr_band_lock"] = raw["nr5g_sa_band_lock"]
	}
	if wanted["samba_switch"] {
		state, e := u.call(ctx, "zwrt_samba", "get_settings", map[string]any{})
		if e != nil {
			return nil, e
		}
		out["samba_switch"] = state["switch"]
	}
	if wanted["web_wifi_nfc_switch"] {
		state, e := u.call(ctx, "zwrt_nfc", "zwrt_nfc_wifi_get", map[string]any{})
		if e != nil {
			return nil, e
		}
		out["web_wifi_nfc_switch"] = state["switch"]
	}
	if wanted["loginfo"] {
		out["loginfo"] = "ok"
	}
	if wanted["sim_slot"] && out["sim_slot"] != nil {
		out["sim_slot"] = strconv.Itoa(intValue(out["sim_slot"]) - 1)
	}
	if has("ppp_status", "cellular_connect_status", "cellular_data_switch", "ConnectionMode", "dial_roam_setting_option") {
		data, e := u.call(ctx, "zwrt_data", "get_wwaniface", map[string]any{"source_module": "web", "cid": 1})
		if e != nil {
			return nil, e
		}
		state := strings.ToLower(textValue(data["connect_status"]))
		connected := state == "connected" || state == "ipv4_ipv6_connected" || state == "ipv4_connected" || state == "ipv6_connected"
		if wanted["ppp_status"] {
			out["ppp_status"] = "ppp_disconnected"
			if connected {
				out["ppp_status"] = "ppp_connected"
			}
		}
		if wanted["cellular_connect_status"] {
			out["cellular_connect_status"] = state
		}
		if wanted["cellular_data_switch"] {
			out["cellular_data_switch"] = "off"
			if intValue(data["enable"]) == 1 {
				out["cellular_data_switch"] = "on"
			}
		}
		if wanted["ConnectionMode"] {
			out["ConnectionMode"] = data["connect_mode"]
		}
		if wanted["dial_roam_setting_option"] {
			out["dial_roam_setting_option"] = data["roam_enable"]
		}
	}
	if has("monthly_tx_bytes", "monthly_rx_bytes", "monthly_time", "month_tx_bytes", "month_rx_bytes", "month_time", "day_tx_bytes", "day_rx_bytes", "total_tx_bytes", "total_rx_bytes", "real_rx_speed", "real_tx_speed", "realtime_rx_thrpt", "realtime_tx_thrpt", "realtime_time") {
		data, e := u.call(ctx, "zwrt_data", "get_wwandst", map[string]any{"source_module": "web", "cid": 1, "type": 4})
		if e != nil {
			return nil, e
		}
		names := map[string]string{"monthly_tx_bytes": "month_tx_bytes", "monthly_rx_bytes": "month_rx_bytes", "monthly_time": "month_time", "realtime_rx_thrpt": "real_rx_speed", "realtime_tx_thrpt": "real_tx_speed", "realtime_time": "real_time"}
		for key := range wanted {
			name := key
			if names[key] != "" {
				name = names[key]
			}
			if value, ok := data[name]; ok {
				out[key] = value
			}
		}
	}
	if has("queryWiFiModuleSwitch", "queryAccessPointInfo") {
		wifi, e := u.wifiSnapshot(ctx)
		if e != nil {
			return nil, e
		}
		for key, value := range wifi {
			out[key] = value
		}
	}
	if has("lan_ipaddr", "lan_netmask", "dhcpEnabled", "dhcpStart", "dhcpEnd", "dhcpLease_hour", "mtu", "tcp_mss") {
		network, e := u.section(ctx, "zwrt_router", "network")
		if e != nil {
			return nil, e
		}
		dhcp, e := u.section(ctx, "zwrt_router", "dhcp")
		if e != nil {
			return nil, e
		}
		data := map[string]any{}
		for key, value := range network {
			data[key] = value
		}
		for key, value := range dhcp {
			data[key] = value
		}

		mappings := map[string][]string{"lan_ipaddr": {"lan_ipaddr", "ipaddr"}, "lan_netmask": {"lan_netmask", "netmask"}, "dhcpStart": {"zte_start", "dhcpStart"}, "dhcpEnd": {"zte_end", "dhcpEnd"}, "mtu": {"mtu"}, "tcp_mss": {"mss", "tcp_mss"}}
		for key, names := range mappings {
			if wanted[key] {
				if value := firstValue(data, names...); value != nil {
					out[key] = value
				}
			}
		}
		if wanted["dhcpEnabled"] {
			if value, ok := data["ignore"]; ok {
				out["dhcpEnabled"] = "1"
				if intValue(value) != 0 {
					out["dhcpEnabled"] = "0"
				}
			}
		}
		if wanted["dhcpLease_hour"] {
			if value := firstValue(data, "leasetime", "dhcpLease_hour"); value != nil {
				lease := textValue(value)
				if strings.HasSuffix(lease, "h") {
					out["dhcpLease_hour"] = lease
				} else {
					out["dhcpLease_hour"] = fmt.Sprintf("%dh", intValue(value)/3600)
				}
			}
		}
	}
	for _, list := range []struct{ key, method, source string }{{"station_list", "router_wireless_access_list", "wireless_access_list_info"}, {"lan_station_list", "router_lan_access_list", "lan_access_list_info"}} {
		if !wanted[list.key] {
			continue
		}
		params := map[string]any{}
		if list.key == "station_list" {
			params["start_id"] = 1
			params["end_id"] = 256
		}
		data, e := u.call(ctx, "zwrt_router.api", list.method, params)
		if e != nil {
			return nil, e
		}
		result := []map[string]any{}
		items, ok := data[list.source].([]any)
		if !ok {
			return nil, fmt.Errorf("unrecognized %s response", list.method)
		}
		{
			for _, item := range items {
				v, ok := item.(map[string]any)
				if !ok {
					continue
				}
				mac := strings.ToUpper(textValue(firstValue(v, "mac_addr", "mac_address")))
				ip := firstValue(v, "ip_addr", "ip_address")
				result = append(result, map[string]any{"mac_addr": mac, "macAddress": mac, "ip_addr": ip, "ipAddress": ip, "hostname": v["hostname"], "hostName": v["hostname"]})
			}
		}
		out[list.key] = result
	}
	if wanted["hostNameList"] {
		data, e := u.call(ctx, "zwrt_router.api", "router_get_modified_lan_hostname", map[string]any{})
		if e != nil {
			return nil, e
		}
		out["hostNameList"] = firstValue(data, "hostNameList", "hostname_list")
		if out["hostNameList"] == nil {
			out["hostNameList"] = []any{}
		}
	}
	if wanted["queryDeviceAccessControlList"] {
		macs, names, e := u.managedAccess(ctx)
		if e != nil {
			return nil, e
		}
		out["AclMode"] = "1"
		out["BlackMacList"] = strings.Join(macs, ";")
		out["BlackNameList"] = strings.Join(names, ";")
	}

	return out, nil
}

func (u *U60UBUS) wifiSnapshot(ctx context.Context) (map[string]any, error) {
	global, err := u.section(ctx, "wireless", "zte_mbb")
	if err != nil {
		return nil, err
	}
	items := []map[string]any{}
	for chip, name := range []string{"main_2g", "main_5g"} {
		values, e := u.section(ctx, "wireless", name)
		if e != nil {
			if chip == 0 {
				return nil, e
			}
			continue
		}
		mode := textValue(values["encryption"])
		auth := "WPA2PSK"
		switch {
		case mode == "none" || mode == "":
			auth = "OPEN"
		case strings.Contains(mode, "sae-mixed"):
			auth = "WPA2PSKWPA3PSK"
		case strings.Contains(mode, "sae"):
			auth = "WPA3PSK"
		}
		enabled := "1"
		if textValue(values["disabled"]) == "1" {
			enabled = "0"
		}
		var maximum any
		for _, key := range []string{"maxassoc", "max_num_sta"} {
			if value := values[key]; textValue(value) != "" {
				maximum = value
				break
			}
		}
		items = append(items, map[string]any{"AccessPointIndex": name, "ChipIndex": chip, "AccessPointSwitchStatus": enabled, "SSID": values["ssid"], "AuthMode": auth, "EncrypType": "AES", "Password": "", "PasswordSet": auth != "OPEN", "ApIsolate": textValue(values["isolate"]), "ApBroadcastDisabled": textValue(values["hidden"]), "ApMaxStationNumber": maximum, "QrImageUrl": fmt.Sprintf("/wifi/qrcode?chip=%d", chip)})
	}
	enabled := textValue(global["wifi_onoff"])
	if enabled == "" {
		enabled = "1"
	}
	return map[string]any{"WiFiModuleSwitch": enabled, "BandSteeringSwitch": textValue(global["lbd"]), "ResponseList": items}, nil
}
