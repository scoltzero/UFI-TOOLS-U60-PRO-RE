// SPDX-License-Identifier: AGPL-3.0-or-later
package device

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"net"
	"strconv"
	"strings"
	"time"
)

var ErrWriteDisabled = errors.New("device writes are disabled")
var ErrInvalidArgument = errors.New("invalid device parameter")

func invalid(field string) error { return fmt.Errorf("%w: %s", ErrInvalidArgument, field) }
func field(v map[string]string, names ...string) string {
	for _, k := range names {
		if s := strings.TrimSpace(v[k]); s != "" {
			return s
		}
	}
	return ""
}
func flagValue(v string) (int, error) {
	switch strings.ToLower(v) {
	case "1", "on", "true", "enable", "enabled", "server":
		return 1, nil
	case "0", "off", "false", "disable", "disabled":
		return 0, nil
	}
	return 0, invalid("expected on/off")
}
func integer(v string, min, max int) (int, error) {
	n, e := strconv.Atoi(v)
	if e != nil || n < min || n > max {
		return 0, invalid("number outside supported range")
	}
	return n, nil
}

func (u *U60UBUS) EnableWrites(enabled bool) { u.allowWrites = enabled }
func (u *U60UBUS) LocalControl() bool        { return u.local != nil }
func (u *U60UBUS) GoFormSet(ctx context.Context, key, value string) error {
	v := map[string]string{"goformId": key, key: value}
	switch key {
	case "net_select", "network_type":
		v = map[string]string{"goformId": "SET_BEARER_PREFERENCE", "BearerPreference": value}
	case "wifi_onoff":
		v = map[string]string{"goformId": "switchWiFiModule", "SwitchOption": value}
	case "sim_slot":
		v = map[string]string{"goformId": "SET_SIM_SLOT", "sim_slot": value}
	case "lte_band_lock":
		v = map[string]string{"goformId": "LTE_BAND_LOCK", "lte_band_lock": value}
	}
	return u.GoFormCommand(ctx, v)
}
func (u *U60UBUS) GoFormCommand(ctx context.Context, v map[string]string) error {
	_, err := u.GoFormCommandResult(ctx, v)
	return err
}
func (u *U60UBUS) GoFormCommandResult(ctx context.Context, v map[string]string) (map[string]any, error) {
	id := strings.ToUpper(v["goformId"])
	if id == "ZWRT_WMS" || id == "SEND_SMS" || id == "DELETE_SMS" || id == "SET_MSG_READ" {
		return u.smsCommand(ctx, v)
	}
	if !u.allowWrites {
		return nil, ErrWriteDisabled
	}
	defer u.invalidateStatus()
	var err error
	write := func(module, procedure string, params map[string]any) error {
		_, e := u.call(ctx, module, procedure, params)
		return e
	}
	switch strings.ToUpper(v["goformId"]) {
	case "CONNECT_NETWORK", "DISCONNECT_NETWORK":
		enable := 0
		if strings.EqualFold(v["goformId"], "CONNECT_NETWORK") {
			enable = 1
		}
		err = write("zwrt_data", "set_wwaniface", map[string]any{"source_module": "web", "cid": 1, "enable": enable})
	case "SET_BEARER_PREFERENCE":
		mode := field(v, "BearerPreference", "net_select")
		if mode == "" || len(mode) > 64 || strings.ContainsAny(mode, "\r\n\x00") {
			return nil, invalid("network mode")
		}
		err = write("zte_nwinfo_api", "nwinfo_set_netselect", map[string]any{"net_select": mode})
	case "SET_CONNECTION_MODE":
		p := map[string]any{"source_module": "web", "cid": 1}
		mode := field(v, "ConnectionMode", "connection_mode")
		switch mode {
		case "auto_dial", "auto":
			mode = "1"
		case "manual_dial", "manual":
			mode = "0"
		}
		if mode != "" {
			n, e := integer(mode, 0, 2)
			if e != nil {
				return nil, e
			}
			p["connect_mode"] = n
		}
		roam := field(v, "roam_setting_option", "dial_roam_setting_option")
		if roam != "" {
			n, e := flagValue(roam)
			if e != nil {
				return nil, e
			}
			p["roam_enable"] = n
		}
		if len(p) == 2 {
			return nil, invalid("connection mode or roaming required")
		}
		err = write("zwrt_data", "set_wwaniface", p)
	case "SWITCHWIFIMODULE":
		enabled, e := flagValue(field(v, "SwitchOption", "wifi_onoff"))
		if e != nil {
			return nil, e
		}
		err = write("zwrt_wlan", "set", map[string]any{"zte_mbb": map[string]string{"wifi_onoff": strconv.Itoa(enabled)}})
	case "SWITCHWIFICHIP":
		chip := field(v, "ChipEnum")
		if chip != "chip1" && chip != "chip2" {
			return nil, invalid("Wi-Fi chip")
		}
		if chip == "chip2" {
			global, e := u.section(ctx, "wireless", "zte_mbb")
			if e != nil {
				return nil, e
			}
			// Disabling steering on this firmware renames the 5 GHz SSID.
			// Do not silently change that setting as part of a band switch.
			if textValue(global["lbd"]) != "0" {
				return nil, ErrUnsupported
			}
		}
		first, second := "0", "1"
		if chip == "chip2" {
			first, second = second, first
		}
		err = write("zwrt_wlan", "set", map[string]any{"zte_mbb": map[string]string{"wifi_onoff": "1"}, "main_2g": map[string]string{"disabled": first}, "main_5g": map[string]string{"disabled": second}})
	case "SETACCESSPOINTINFO":
		err = u.setAccessPoint(ctx, v)
	case "DHCP_SETTING":
		err = u.setLAN(ctx, v)
	case "SETDEVICEACCESSCONTROLLIST":
		err = u.setManagedAccess(ctx, v)
	case "EDIT_HOSTNAME":
		mac, e := net.ParseMAC(field(v, "mac", "macAddress"))
		if e != nil {
			return nil, invalid("MAC address")
		}
		name := field(v, "hostname")
		if len(name) > 63 || strings.ContainsAny(name, "\r\n\x00") {
			return nil, invalid("hostname")
		}
		err = write("zwrt_router.api", "router_modify_lan_hostname", map[string]any{"mac": mac.String(), "hostname": name})
	case "LTE_BAND_LOCK":
		band := field(v, "lte_band_lock", "lte_band")
		mask := new(big.Int)
		if strings.HasPrefix(band, "0x") {
			if _, ok := mask.SetString(band[2:], 16); !ok || mask.Sign() <= 0 || mask.BitLen() > 256 {
				return nil, invalid("LTE band mask")
			}
		} else {
			for _, item := range strings.Split(band, ",") {
				n, e := integer(item, 1, 256)
				if e != nil {
					return nil, invalid("LTE bands")
				}
				mask.SetBit(mask, n-1, 1)
			}
		}
		band = "0x" + mask.Text(16)
		err = write("zte_nwinfo_api", "nwinfo_set_gwl_bandlock", map[string]any{"is_lte_band": "1", "lte_band_mask": band, "is_gw_band": "0", "gw_band_mask": "0"})
	case "NR_BAND_LOCK":
		band := field(v, "nr5g_sa_band_lock", "nr_band_lock", "nr_band")
		if len(band) > 512 {
			return nil, invalid("NR bands")
		}
		for _, item := range strings.Split(band, ",") {
			if _, err := integer(item, 1, 261); err != nil {
				return nil, invalid("NR bands")
			}
		}
		err = write("zte_nwinfo_api", "nwinfo_set_sa_bandlock", map[string]any{"nr5g_sa_band_lock": band})
	case "CELL_LOCK":
		rat, e := integer(field(v, "rat"), 0, 1)
		if e != nil {
			return nil, e
		}
		maxPCI := 503
		if rat == 1 {
			maxPCI = 1007
		}
		pci, e := integer(field(v, "pci"), 0, maxPCI)
		if e != nil {
			return nil, e
		}
		arfcn, e := integer(field(v, "earfcn"), 0, 3279165)
		if e != nil {
			return nil, e
		}
		if rat == 0 {
			err = write("zte_nwinfo_api", "nwinfo_lock_lte_cell", map[string]any{"lock_lte_pci": strconv.Itoa(pci), "lock_lte_earfcn": strconv.Itoa(arfcn)})
		} else {
			band, e := integer(field(v, "band"), 1, 261)
			if e != nil {
				return nil, invalid("NR band is required")
			}
			scs := field(v, "scs")
			if scs != "15" && scs != "30" && scs != "60" && scs != "120" {
				return nil, invalid("NR subcarrier spacing")
			}
			err = write("zte_nwinfo_api", "nwinfo_lock_nr_cell2", map[string]any{"lock_nr_pci": strconv.Itoa(pci), "lock_nr_earfcn": strconv.Itoa(arfcn), "lock_nr_cell_band": strconv.Itoa(band), "lock_nr_cell_scs": scs})
		}
	case "UNLOCK_ALL_CELL":
		err = write("zte_nwinfo_api", "nwinfo_reset_band_cell_setting", map[string]any{})
	case "SET_SIM_SLOT":
		slot, e := integer(field(v, "sim_slot"), 0, 1)
		if e != nil {
			return nil, e
		}
		err = write("zwrt_zte_mdm.api", "sim_switch_slot", map[string]any{"slot": slot + 1})
	case "PERFORMANCE_MODE", "PERFORMANCE_MODE_SETTING":
		err = u.setPerformance(field(v, "performance_mode"))
	case "INDICATOR_LIGHT_SETTING":
		err = u.setLights(field(v, "indicator_light_switch"))
	case "REBOOT_DEVICE":
		err = write("zwrt_mc.device.manager", "device_reboot", map[string]any{"moduleName": "web"})
	case "SHUTDOWN_DEVICE":
		err = write("zwrt_mc.device.manager", "device_poweroff", map[string]any{"moduleName": "web"})
	case "SAMBA_SETTING":
		enabled, e := flagValue(field(v, "samba_switch"))
		if e != nil {
			return nil, e
		}
		err = write("zwrt_samba", "set_settings", map[string]any{"switch": strconv.Itoa(enabled)})
	case "WIFI_NFC_SET":
		enabled, e := flagValue(field(v, "web_wifi_nfc_switch"))
		if e != nil {
			return nil, e
		}
		err = write("zwrt_nfc", "zwrt_nfc_wifi_set", map[string]any{"switch": enabled, "flag": 0})
	case "APN_PROC_EX":
		err = u.apnCommand(ctx, v)
	default:
		return nil, ErrUnsupported
	}
	if err != nil {
		return nil, err
	}
	u.invalidateStatus()
	return map[string]any{"result": "success", "requires_reconnect": strings.EqualFold(v["goformId"], "DHCP_SETTING")}, nil
}

func (u *U60UBUS) setAccessPoint(ctx context.Context, v map[string]string) error {
	index := field(v, "AccessPointIndex")
	if index == "" || index == "0" {
		index = "main_2g"
		if v["ChipIndex"] == "1" {
			index = "main_5g"
		}
	}
	if index == "1" {
		index = "main_5g"
	}
	if index != "main_2g" && index != "main_5g" {
		return invalid("access point")
	}
	ssid := v["SSID"]
	if len(ssid) == 0 || len(ssid) > 32 || strings.ContainsAny(ssid, "\x00\r\n") {
		return invalid("SSID must contain 1–32 bytes")
	}
	modes := map[string]string{"OPEN": "none", "WPA2PSK": "psk2+ccmp", "WPA3PSK": "sae", "WPA2PSKWPA3PSK": "sae-mixed"}
	encryption, ok := modes[v["AuthMode"]]
	if !ok {
		return invalid("Wi-Fi authentication mode")
	}
	values := map[string]any{"ssid": ssid, "encryption": encryption}
	if v["Password"] != "" {
		password, e := base64.StdEncoding.DecodeString(v["Password"])
		if e != nil || len(password) < 8 || len(password) > 63 {
			return invalid("Wi-Fi passphrase must contain 8–63 bytes")
		}
		values["key"] = string(password)
	}
	if value := field(v, "AccessPointSwitchStatus"); value != "" {
		enabled, e := flagValue(value)
		if e != nil {
			return e
		}
		values["disabled"] = strconv.Itoa(1 - enabled)
	}
	if value := field(v, "ApBroadcastDisabled"); value != "" {
		n, e := flagValue(value)
		if e != nil {
			return e
		}
		values["hidden"] = strconv.Itoa(n)
	}
	if value := field(v, "ApIsolate"); value != "" {
		n, e := flagValue(value)
		if e != nil {
			return e
		}
		values["isolate"] = strconv.Itoa(n)
	}
	if value := field(v, "ApMaxStationNumber"); value != "" {
		n, e := integer(value, 1, 128)
		if e != nil {
			return e
		}
		values["maxassoc"] = strconv.Itoa(n)
	}
	before, e := u.section(ctx, "wireless", index)
	if e != nil {
		return e
	}
	if encryption != "none" && values["key"] == nil && textValue(before["key"]) == "" {
		return invalid("a passphrase is required for protected Wi-Fi")
	}
	// A blank password means preserve it, never fetch it into the browser.
	global, e := u.section(ctx, "wireless", "zte_mbb")
	if e != nil {
		return e
	}
	enabled := textValue(global["wifi_onoff"])
	if enabled != "0" && enabled != "1" {
		return invalid("Wi-Fi module state unavailable")
	}
	return u.applyAccessPoint(ctx, index, values, enabled == "0")
}

func (u *U60UBUS) applyAccessPoint(ctx context.Context, section string, values map[string]any, offline bool) error {
	if offline && u.local == nil {
		return ErrUnsupported
	}
	before, err := u.section(ctx, "wireless", section)
	if err != nil {
		return err
	}
	// On this firmware, rpcd UCI commit emits a change event that can enable
	// Wi-Fi. While off, use local UCI persistence without that event or reload.
	write := func(method string, options map[string]any) error {
		if !offline {
			params := map[string]any{"config": "wireless"}
			if method == "set" {
				params["section"], params["values"] = section, options
			}
			if method == "delete" {
				params["section"], params["option"] = section, options["option"]
			}
			_, e := u.call(ctx, "uci", method, params)
			return e
		}
		if method == "set" {
			for key, value := range options {
				if _, e := u.local.Runner.Run(ctx, 5*time.Second, "uci", "set", "wireless."+section+"."+key+"="+textValue(value)); e != nil {
					return e
				}
			}
			return nil
		}
		path := "wireless"
		if method == "delete" {
			path += "." + section + "." + textValue(options["option"])
		}
		_, e := u.local.Runner.Run(ctx, 5*time.Second, "uci", method, path)
		return e
	}
	reload := func() error {
		if offline {
			return nil
		}
		_, e := u.call(ctx, "zwrt_wlan", "reload", map[string]any{})
		return e
	}
	rollback := func() error {
		var failures []error
		current, readErr := u.section(ctx, "wireless", section)
		if readErr != nil {
			failures = append(failures, readErr)
		}
		old := map[string]any{}
		for key := range values {
			if value, ok := before[key]; ok {
				old[key] = value
			} else if _, exists := current[key]; exists {
				failures = append(failures, write("delete", map[string]any{"option": key}))
			}
		}
		failures = append(failures, write("set", old), write("commit", nil), reload())
		return errors.Join(failures...)
	}
	for _, action := range []func() error{func() error { return write("set", values) }, func() error { return write("commit", nil) }, reload} {
		if err = action(); err != nil {
			if restoreErr := rollback(); restoreErr != nil {
				return fmt.Errorf("%w; rollback incomplete: %v", err, restoreErr)
			}
			return err
		}
	}
	return nil
}

func (u *U60UBUS) setLAN(ctx context.Context, v map[string]string) error {
	ip := net.ParseIP(field(v, "lanIp", "lan_ipaddr", "ipaddr")).To4()
	maskIP := net.ParseIP(field(v, "netmask", "lanNetmask", "lan_netmask", "subnetMask")).To4()
	if ip == nil || maskIP == nil {
		return invalid("IPv4 address/netmask")
	}
	mask := net.IPMask(maskIP)
	ones, bits := mask.Size()
	if bits != 32 || ones < 1 || ones > 30 {
		return invalid("contiguous IPv4 subnet mask")
	}
	network := &net.IPNet{IP: ip.Mask(mask), Mask: mask}
	broadcast := make(net.IP, 4)
	for i := range ip {
		broadcast[i] = network.IP[i] | ^mask[i]
	}
	if ip.Equal(network.IP) || ip.Equal(broadcast) {
		return invalid("gateway is a network/broadcast address")
	}
	enabled, err := flagValue(field(v, "dhcpEnabled", "dhcpServer", "lanDhcpType"))
	if err != nil {
		return err
	}
	p := map[string]any{"ipaddr": ip.String(), "netmask": net.IP(maskIP).String(), "ignore": 1 - enabled}
	if enabled == 1 {
		start := net.ParseIP(field(v, "dhcpStart")).To4()
		end := net.ParseIP(field(v, "dhcpEnd")).To4()
		if start == nil || end == nil || !network.Contains(start) || !network.Contains(end) || start.Equal(network.IP) || end.Equal(broadcast) {
			return invalid("DHCP range")
		}
		integerIP := func(v net.IP) uint32 { return uint32(v[0])<<24 | uint32(v[1])<<16 | uint32(v[2])<<8 | uint32(v[3]) }
		a, b, g := integerIP(start), integerIP(end), integerIP(ip)
		if a > b || g >= a && g <= b {
			return invalid("DHCP range order/gateway overlap")
		}
		lease, err := integer(strings.TrimSuffix(field(v, "dhcpLease", "dhcpLease_hour"), "h"), 1, 8760)
		if err != nil {
			return err
		}
		p["zte_start"], p["zte_end"], p["leasetime"] = start.String(), end.String(), strconv.Itoa(lease*3600)
	}
	_, err = u.call(ctx, "zwrt_router.api", "router_set_lan_para", p)
	return err
}

func (u *U60UBUS) apnCommand(ctx context.Context, v map[string]string) error {
	mode := field(v, "apn_mode")
	action := field(v, "apn_action")
	if mode == "auto" {
		_, err := u.call(ctx, "zwrt_apn_object", "set_apn_mode", map[string]any{"apn_mode": 0})
		return err
	}
	id := field(v, "index", "profileId")
	if action == "delete" {
		if id == "" {
			return invalid("APN profile identifier")
		}
		_, err := u.call(ctx, "zwrt_apn_object", "delete_manu_apn", map[string]any{"profileId": id})
		return err
	}
	if action == "set_default" {
		if id == "" {
			return invalid("APN profile identifier")
		}
		return SelectAPN(ctx, u, "manual", id)
	}
	if action != "save" {
		return invalid("APN action")
	}
	name, apn := field(v, "profile_name", "profilename"), field(v, "apn_wan_apn", "wanapn")
	if name == "" || len(name) > 64 || apn == "" || len(apn) > 100 {
		return invalid("APN profile name/APN")
	}
	pdpText := field(v, "apn_pdp_type", "pdpType")
	if mapped := map[string]string{"IP": "1", "IPv6": "2", "IPv4v6": "3"}[pdpText]; mapped != "" {
		pdpText = mapped
	}
	pdp, e := integer(pdpText, 1, 3)
	if e != nil {
		return e
	}
	authText := field(v, "apn_ppp_auth_mode", "pppAuthMode")
	if mapped := map[string]string{"none": "0", "pap": "1", "chap": "2", "auto": "3"}[strings.ToLower(authText)]; mapped != "" {
		authText = mapped
	}
	auth, e := integer(authText, 0, 3)
	if e != nil {
		return e
	}
	if auth > 0 && (field(v, "apn_ppp_username", "username") == "" || v["apn_ppp_passwd"] == "") {
		return invalid("authenticated APN requires newly entered credentials")
	}
	if v["apn_ppp_passwd"] != "" && u.local == nil {
		return invalid("credential-bearing APN requires local backend")
	}
	params := map[string]any{"profilename": name, "wanapn": apn, "pdpType": pdp, "pppAuthMode": auth, "username": field(v, "apn_ppp_username", "username"), "password": v["apn_ppp_passwd"]}
	method := "add_manu_apn"
	if id != "" && id != "-1" {
		method = "modify_manu_apn"
		params["profileId"] = id
	}
	_, e = u.call(ctx, "zwrt_apn_object", method, params)
	return e
}
