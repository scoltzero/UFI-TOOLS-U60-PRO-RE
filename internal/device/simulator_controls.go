// SPDX-License-Identifier: AGPL-3.0-or-later
package device

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
)

func (s *Simulator) initControls() {
	if s.apns == nil {
		s.apns = map[string]map[string]any{"1": {"profileId": "1", "profilename": "模拟配置", "wanapn": "internet", "pdpType": 3, "pppAuthMode": 0}}
		s.apnID = "1"
		s.apnNext = 2
	}
	if s.wifi == nil {
		s.wifi = []map[string]any{}
		for i, name := range []string{"UFI-Simulator-2G", "UFI-Simulator-5G"} {
			s.wifi = append(s.wifi, map[string]any{"AccessPointIndex": []string{"main_2g", "main_5g"}[i], "ChipIndex": i, "AccessPointSwitchStatus": "1", "SSID": name, "AuthMode": "WPA2PSK", "EncrypType": "AES", "Password": "", "PasswordSet": true, "ApIsolate": "0", "ApBroadcastDisabled": "0", "ApMaxStationNumber": 32, "QrImageUrl": "/wifi/qrcode?chip=" + strconv.Itoa(i)})
		}
	}
	if s.hostnames == nil {
		s.hostnames = map[string]string{}
	}
}
func (s *Simulator) LocalControl() bool { return true }
func (s *Simulator) Call(_ context.Context, module, procedure string, p map[string]any) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initControls()
	if module != "zwrt_apn_object" {
		return nil, ErrUnsupported
	}
	switch procedure {
	case "get_auto_apn_list":
		return map[string]any{"apnListArray": []any{map[string]any{"profileId": "auto", "profilename": "模拟自动配置", "wanapn": "internet", "pdpType": 3, "pppAuthMode": 0}}}, nil
	case "get_manu_apn_list":
		list := []any{}
		for _, v := range s.apns {
			copy := map[string]any{}
			for k, x := range v {
				copy[k] = x
			}
			list = append(list, copy)
		}
		return map[string]any{"apnListArray": list}, nil
	case "get_apn_mode":
		return map[string]any{"apn_mode": s.apnMode}, nil
	case "get_enabled_manu_apn_id":
		return map[string]any{"profileId": s.apnID}, nil
	case "set_apn_mode":
		s.apnMode = intValue(p["apn_mode"])
		s.values["apn_mode"] = "auto"
		if s.apnMode == 1 {
			s.values["apn_mode"] = "manual"
		}
	case "add_manu_apn", "modify_manu_apn":
		id := textValue(p["profileId"])
		if procedure == "add_manu_apn" {
			id = strconv.Itoa(s.apnNext)
			s.apnNext++
		} else if s.apns[id] == nil {
			return nil, errors.New("APN profile does not exist")
		}
		copy := map[string]any{"profileId": id}
		for k, v := range p {
			copy[k] = v
		}
		s.apns[id] = copy
	case "delete_manu_apn":
		id := textValue(p["profileId"])
		if s.apns[id] == nil {
			return nil, errors.New("APN profile does not exist")
		}
		delete(s.apns, id)
		if s.apnID == id {
			s.apnID = ""
			s.apnMode = 0
			s.values["apn_mode"] = "auto"
		}
	case "enable_manu_apn_id":
		id := textValue(p["profileId"])
		if s.apns[id] == nil {
			return nil, errors.New("APN profile does not exist")
		}
		s.apnID = id
	case "get_candidate_apn_by_cid":
		return s.apns[s.apnID], nil
	default:
		return nil, ErrUnsupported
	}
	return map[string]any{"result": "success", "simulated": true}, nil
}
func (s *Simulator) Snapshot(_ context.Context, keys []string) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initControls()
	out := map[string]any{}
	for _, key := range keys {
		if v, ok := s.values[key]; ok {
			out[key] = v
		}
		switch key {
		case "queryWiFiModuleSwitch", "queryAccessPointInfo":
			enabled := s.values["wifi_onoff"]
			if enabled == "" {
				enabled = "1"
			}
			out["WiFiModuleSwitch"] = enabled
			out["BandSteeringSwitch"] = "0"
			list := []map[string]any{}
			for _, v := range s.wifi {
				copy := map[string]any{}
				for k, x := range v {
					copy[k] = x
				}
				list = append(list, copy)
			}
			out["ResponseList"] = list
		case "station_list", "lan_station_list", "hostNameList":
			out[key] = []any{}
		case "queryDeviceAccessControlList":
			out["AclMode"] = "1"
			out["BlackMacList"] = strings.Join(s.blacklist, ";")
			out["BlackNameList"] = ""
		}
	}
	return out, nil
}
func (s *Simulator) simulateCommand(v map[string]string) (map[string]any, error) {
	id := strings.ToUpper(v["goformId"])
	if id == "ZWRT_WMS" || id == "SEND_SMS" || id == "DELETE_SMS" || id == "SET_MSG_READ" {
		return s.smsCommand(v)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initControls()
	switch strings.ToUpper(v["goformId"]) {
	case "CONNECT_NETWORK":
		s.values["ppp_status"] = "ppp_connected"
	case "DISCONNECT_NETWORK":
		s.values["ppp_status"] = "ppp_disconnected"
	case "SET_BEARER_PREFERENCE":
		s.values["net_select"] = v["BearerPreference"]
	case "SWITCHWIFIMODULE":
		s.values["wifi_onoff"] = v["SwitchOption"]
	case "SWITCHWIFICHIP":
		for i, p := range s.wifi {
			p["AccessPointSwitchStatus"] = "0"
			if v["ChipEnum"] == "chip"+strconv.Itoa(i+1) {
				p["AccessPointSwitchStatus"] = "1"
			}
		}
		s.values["wifi_onoff"] = "1"
	case "SETACCESSPOINTINFO":
		chip := intValue(v["ChipIndex"])
		if v["AccessPointIndex"] == "main_5g" {
			chip = 1
		}
		if chip < 0 || chip >= len(s.wifi) || len(v["SSID"]) == 0 || len(v["SSID"]) > 32 {
			return nil, invalid("Wi-Fi access point/SSID")
		}
		p := s.wifi[chip]
		if v["Password"] != "" {
			b, err := base64.StdEncoding.DecodeString(v["Password"])
			if err != nil || len(b) < 8 || len(b) > 63 {
				return nil, invalid("Wi-Fi password")
			}
		}
		for _, key := range []string{"SSID", "AuthMode", "ApBroadcastDisabled", "ApIsolate"} {
			if value, ok := v[key]; ok {
				p[key] = value
			}
		}
	case "SETDEVICEACCESSCONTROLLIST":
		s.blacklist = nil
		for _, mac := range strings.Split(v["BlackMacList"], ";") {
			if mac != "" {
				s.blacklist = append(s.blacklist, mac)
			}
		}
	case "EDIT_HOSTNAME":
		s.hostnames[v["mac"]] = v["hostname"]
	case "DHCP_SETTING":
		for _, key := range []string{"lanIp", "lan_ipaddr", "lan_netmask", "dhcpEnabled", "dhcpStart", "dhcpEnd", "dhcpLease_hour"} {
			if x, ok := v[key]; ok {
				s.values[key] = x
			}
		}
		if v["lanIp"] != "" {
			s.values["lan_ipaddr"] = v["lanIp"]
		}
	}
	for key, value := range v {
		if key != "goformId" && key != "isTest" {
			s.values[key] = value
		}
	}
	return map[string]any{"result": "success", "simulated": true}, nil
}
