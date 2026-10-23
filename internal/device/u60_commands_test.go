package device

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type rpcRecord struct {
	module, method string
	params         map[string]any
}
type fakeLocalUBUS struct {
	calls      []rpcRecord
	section    map[string]any
	failReload bool
	failCommit bool
	reject     bool
	connected  bool
}

func (f *fakeLocalUBUS) Run(_ context.Context, _ time.Duration, name string, args ...string) ([]byte, error) {
	if name == "uci" {
		f.calls = append(f.calls, rpcRecord{"uci-cli", args[0], nil})
		switch args[0] {
		case "set":
			path, value, _ := strings.Cut(args[1], "=")
			parts := strings.Split(path, ".")
			f.section[parts[len(parts)-1]] = value
		case "delete":
			parts := strings.Split(args[1], ".")
			delete(f.section, parts[len(parts)-1])
		case "commit":
			if f.failCommit {
				f.failCommit = false
				return nil, errors.New("commit failed")
			}
		default:
			return nil, errors.New("unexpected UCI method")
		}
		return nil, nil
	}
	if name != "ubus" || len(args) < 3 || args[0] != "call" {
		return nil, errors.New("unexpected executable")
	}
	p := map[string]any{}
	if len(args) > 3 {
		if err := json.Unmarshal([]byte(args[3]), &p); err != nil {
			return nil, err
		}
	}
	f.calls = append(f.calls, rpcRecord{args[1], args[2], p})
	result := map[string]any{}
	switch args[2] {
	case "get_device_info":
		result = map[string]any{"device_model": "MU5250"}
	case "nwinfo_get_netinfo":
		result = map[string]any{"network_type": "5G SA"}
	case "get_sim_info_before":
		result = map[string]any{"sim_states": "sim ready"}
	case "router_get_status_no_auth":
		state := "disconnected"
		if f.connected {
			state = "connected"
		}
		result = map[string]any{"current_wan_status": state}
	case "get_wwaniface":
		state := "disconnected"
		if f.connected {
			state = "connected"
		}
		result = map[string]any{"connect_status": state, "enable": 0}
	case "set_wwaniface":
		if f.reject {
			result = map[string]any{"error": 2}
		} else {
			f.connected = intValue(p["enable"]) == 1
		}
	case "get":
		copy := map[string]any{}
		for k, v := range f.section {
			copy[k] = v
		}
		result = map[string]any{"values": copy}
	case "set":

		if values, ok := p["values"].(map[string]any); ok {
			for k, v := range values {
				f.section[k] = v
			}
		}
	case "delete":
		delete(f.section, textValue(p["option"]))
	case "reload":
		if f.failReload {
			f.failReload = false
			result = map[string]any{"error": 1}
		}
	}
	return json.Marshal(result)
}
func localFixture(allowed bool) (*U60UBUS, *fakeLocalUBUS) {
	u := NewU60Local(allowed)
	f := &fakeLocalUBUS{section: map[string]any{"ssid": "existing-ssid", "key": "fixture-existing-password", "encryption": "psk2+ccmp", "disabled": "0", "wifi_onoff": "1"}}
	u.local.Runner = f
	return u, f
}

func TestLocalControlsAreOptInAndUseTypedRPC(t *testing.T) {
	u, f := localFixture(false)
	if _, err := u.GoFormCommandResult(t.Context(), map[string]string{"goformId": "CONNECT_NETWORK"}); !errors.Is(err, ErrWriteDisabled) || len(f.calls) != 0 {
		t.Fatal("disabled write executed")
	}
	u.EnableWrites(true)
	if _, err := u.GoFormCommandResult(t.Context(), map[string]string{"goformId": "CONNECT_NETWORK"}); err != nil {
		t.Fatal(err)
	}
	call := f.calls[0]
	if call.module != "zwrt_data" || call.method != "set_wwaniface" || call.params["enable"] != float64(1) || call.params["cid"] != float64(1) {
		t.Fatalf("RPC %#v", call)
	}
	f.reject = true
	if _, err := u.GoFormCommandResult(t.Context(), map[string]string{"goformId": "DISCONNECT_NETWORK"}); err == nil {
		t.Fatal("numeric service error was reported as success")
	}
}

func TestLocalInvalidParametersDoNotWrite(t *testing.T) {
	u, f := localFixture(true)
	cases := []map[string]string{
		{"goformId": "switchWiFiChip", "ChipEnum": "dual"},
		{"goformId": "CELL_LOCK", "rat": "1", "pci": "1008", "earfcn": "123"},
		{"goformId": "setAccessPointInfo", "SSID": "", "AuthMode": "WPA2PSK"},
		{"goformId": "setAccessPointInfo", "SSID": "test", "AuthMode": "WPA2PSK", "Password": base64.StdEncoding.EncodeToString([]byte("short"))},
		{"goformId": "DHCP_SETTING", "lanIp": "192.168.8.1", "netmask": "255.255.255.0", "dhcpEnabled": "1", "dhcpStart": "192.168.9.2", "dhcpEnd": "192.168.9.20", "dhcpLease": "12"},
	}
	for _, c := range cases {
		if _, err := u.GoFormCommandResult(t.Context(), c); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("%s invalid input accepted: %v", c["goformId"], err)
		}
	}
	if len(f.calls) != 0 {
		t.Fatalf("invalid input reached device: %#v", f.calls)
	}
}

func TestWiFiBandSwitchDoesNotSilentlyChangeSteering(t *testing.T) {
	for _, chip := range []string{"chip1", "chip2"} {
		t.Run(chip, func(t *testing.T) {
			u, f := localFixture(true)
			f.section["lbd"] = "0"
			if _, err := u.GoFormCommandResult(t.Context(), map[string]string{"goformId": "switchWiFiChip", "ChipEnum": chip}); err != nil {
				t.Fatal(err)
			}
			call := f.calls[len(f.calls)-1]
			if call.module != "zwrt_wlan" || call.method != "set" {
				t.Fatal("missing native switch")
			}
			global := call.params["zte_mbb"].(map[string]any)
			if len(global) != 1 || global["wifi_onoff"] != "1" {
				t.Fatal("switch altered steering")
			}
			first, second := "0", "1"
			if chip == "chip2" {
				first, second = second, first
			}
			for section, disabled := range map[string]string{"main_2g": first, "main_5g": second} {
				v := call.params[section].(map[string]any)
				if len(v) != 1 || v["disabled"] != disabled {
					t.Fatal("switch altered other AP options")
				}
			}
		})
	}
	for _, steering := range []string{"1", ""} {
		u, f := localFixture(true)
		f.section["lbd"] = steering
		if _, err := u.GoFormCommandResult(t.Context(), map[string]string{"goformId": "switchWiFiChip", "ChipEnum": "chip2"}); !errors.Is(err, ErrUnsupported) {
			t.Fatal("unsafe 5 GHz conversion accepted")
		}
		for _, call := range f.calls {
			if call.method != "get" {
				t.Fatal("rejected conversion reached a write")
			}
		}
	}
}

func TestWiFiGlobalEnablePreservesBandConfiguration(t *testing.T) {
	u, f := localFixture(true)
	f.section["lbd"] = "1"
	if _, err := u.GoFormCommandResult(t.Context(), map[string]string{"goformId": "switchWiFiModule", "SwitchOption": "1"}); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 || len(f.calls[0].params) != 1 {
		t.Fatal("global enable changed AP settings")
	}
	options := f.calls[0].params["zte_mbb"].(map[string]any)
	if len(options) != 1 || options["wifi_onoff"] != "1" {
		t.Fatal("global enable changed steering")
	}
}

func TestWiFiRollbackRestoresOnlyChangedValues(t *testing.T) {
	u, f := localFixture(true)
	f.section["wifi_onoff"] = "0"
	f.failCommit = true
	_, err := u.GoFormCommandResult(t.Context(), map[string]string{"goformId": "setAccessPointInfo", "AccessPointIndex": "main_2g", "SSID": "new-ssid", "AuthMode": "WPA2PSK", "ApMaxStationNumber": "16"})
	if err == nil {
		t.Fatal("commit error hidden")
	}
	if f.section["ssid"] != "existing-ssid" || f.section["key"] != "fixture-existing-password" {
		t.Fatalf("rollback changed existing configuration: %#v", f.section)
	}
	if _, exists := f.section["maxassoc"]; exists || f.section["wifi_onoff"] != "0" {
		t.Fatal("rollback retained a newly introduced option or changed the switch")
	}
	for _, c := range f.calls {
		if c.method == "set" {
			if values, ok := c.params["values"].(map[string]any); ok {
				if _, wrote := values["key"]; wrote {
					t.Fatal("blank password rewrote the existing password")
				}
			}
		}
	}
}

func TestWiFiSaveWhileOffDoesNotReloadOrChangeSwitch(t *testing.T) {
	u, f := localFixture(true)
	f.section["wifi_onoff"] = "0"
	_, err := u.GoFormCommandResult(t.Context(), map[string]string{"goformId": "setAccessPointInfo", "AccessPointIndex": "main_2g", "SSID": "saved-while-off", "AuthMode": "WPA2PSK"})
	if err != nil {
		t.Fatal(err)
	}
	applied := false
	for _, c := range f.calls {
		if c.module == "zwrt_wlan" || (c.module == "uci" && c.method != "get") {
			t.Fatal("saving while off used the path that re-enables Wi-Fi")
		}
		if c.module == "uci-cli" && c.method == "commit" {
			applied = true
		}
	}

	if !applied || f.section["ssid"] != "saved-while-off" || f.section["wifi_onoff"] != "0" || f.section["key"] != "fixture-existing-password" {
		t.Fatal("offline save did not preserve the existing state")
	}
}

func TestWiFiOnlineReloadFailureRestoresSettings(t *testing.T) {
	u, f := localFixture(true)
	f.failReload = true
	_, err := u.GoFormCommandResult(t.Context(), map[string]string{"goformId": "setAccessPointInfo", "AccessPointIndex": "main_2g", "SSID": "new-ssid", "AuthMode": "WPA2PSK"})
	if err == nil || f.section["ssid"] != "existing-ssid" || f.section["wifi_onoff"] != "1" || f.section["key"] != "fixture-existing-password" {
		t.Fatal("online reload failure was hidden or rollback changed existing settings")
	}
}

func TestWiFiSaveRequiresKnownModuleStateBeforeWriting(t *testing.T) {
	u, f := localFixture(true)
	delete(f.section, "wifi_onoff")
	_, err := u.GoFormCommandResult(t.Context(), map[string]string{"goformId": "setAccessPointInfo", "SSID": "new-ssid", "AuthMode": "WPA2PSK"})
	if err == nil {
		t.Fatal("unknown module state accepted")
	}
	for _, c := range f.calls {
		if c.method != "get" {
			t.Fatal("unknown state reached a write")
		}
	}
}
func TestStructuredWiFiSnapshotDoesNotExposeSavedPassword(t *testing.T) {
	u, _ := localFixture(false)
	v, err := u.Snapshot(t.Context(), []string{"queryWiFiModuleSwitch", "queryAccessPointInfo"})
	if err != nil {
		t.Fatal(err)
	}
	list, ok := v["ResponseList"].([]map[string]any)
	if !ok || len(list) != 2 {
		t.Fatalf("structured snapshot: %#v", v)
	}
	if list[0]["Password"] != "" || list[0]["SSID"] != "existing-ssid" {
		t.Fatalf("unsafe snapshot: %#v", list[0])
	}
}
func TestDisconnectedStateAndWriteInvalidateCache(t *testing.T) {
	u, f := localFixture(true)
	before, err := u.Status(t.Context())
	if err != nil || before.Connected {
		t.Fatalf("disconnected was considered connected: %#v %v", before, err)
	}
	if _, err = u.GoFormCommandResult(t.Context(), map[string]string{"goformId": "CONNECT_NETWORK"}); err != nil {
		t.Fatal(err)
	}
	after, err := u.Status(t.Context())
	if err != nil || !after.Connected || !f.connected {
		t.Fatalf("stale cache after write: %#v %v", after, err)
	}
}

func TestUnavailableWiFiStationLimitIsNotInventedOrWritten(t *testing.T) {
	u, f := localFixture(true)
	f.section["maxassoc"] = ""
	v, err := u.wifiSnapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	list := v["ResponseList"].([]map[string]any)
	if list[0]["ApMaxStationNumber"] != nil {
		t.Fatalf("unavailable limit became a reading: %v", list[0]["ApMaxStationNumber"])
	}
	_, err = u.GoFormCommandResult(t.Context(), map[string]string{"goformId": "setAccessPointInfo", "AccessPointIndex": "main_2g", "SSID": "new-test-ssid", "AuthMode": "WPA2PSK", "ApMaxStationNumber": ""})
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range f.calls {
		if call.method == "set" {
			values, _ := call.params["values"].(map[string]any)
			if _, ok := values["maxassoc"]; ok {
				t.Fatal("blank station limit rewrote the existing option")
			}
		}
	}
	f.section["max_num_sta"] = "16"
	v, err = u.wifiSnapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	list = v["ResponseList"].([]map[string]any)
	if list[0]["ApMaxStationNumber"] != "16" {
		t.Fatal("available firmware limit was lost")
	}
}
