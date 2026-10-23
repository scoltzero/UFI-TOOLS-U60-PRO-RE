package device

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type accessFixture struct {
	rules     map[string]any
	fail      bool
	mutations int
	reloads   int
}

func (f *accessFixture) Run(_ context.Context, _ time.Duration, name string, args ...string) ([]byte, error) {
	if name == "/etc/init.d/firewall" {
		f.reloads++
		if f.fail {
			f.fail = false
			return nil, errors.New("fixture reload failed")
		}
		return nil, nil
	}
	if name != "ubus" || len(args) < 3 {
		return nil, errors.New("unexpected command")
	}
	p := map[string]any{}
	if len(args) > 3 {
		_ = json.Unmarshal([]byte(args[3]), &p)
	}
	switch args[2] {
	case "get":
		cp := map[string]any{}
		for k, v := range f.rules {
			cp[k] = v
		}
		return json.Marshal(map[string]any{"values": cp})
	case "add":
		f.mutations++
		f.rules[textValue(p["name"])] = p["values"]
	case "delete":
		f.mutations++
		delete(f.rules, textValue(p["section"]))
	}
	return []byte(`{}`), nil
}
func TestManagedInternetRulesPreserveOtherOwnersAndRollback(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(textValue(fail), func(t *testing.T) {
			existing := map[string]any{".type": "zone", "name": "lan", "network": []any{"lan"}}
			user := map[string]any{".type": "rule", "name": "user-rule", "target": "ACCEPT"}
			oldID := accessRuleID("00:11:22:33:44:55")
			f := &accessFixture{rules: map[string]any{"lan": existing, "user": user, oldID: map[string]any{"ufi_tools_re": "1", "src_mac": "00:11:22:33:44:55", "target": "REJECT"}}, fail: fail}
			u := NewU60Local(true)
			u.local.Runner = f
			err := u.setManagedAccess(t.Context(), map[string]string{"AclMode": "1", "BlackMacList": "AA:BB:CC:DD:EE:FF", "BlackNameList": "test-client"})
			if (err != nil) != fail {
				t.Fatalf("unexpected result: %v", err)
			}
			if f.rules["user"] == nil || f.rules["lan"] == nil {
				t.Fatal("unrelated rules lost")
			}
			if fail {
				if f.rules[oldID] == nil || f.rules[accessRuleID("AA:BB:CC:DD:EE:FF")] != nil {
					t.Fatal("rollback did not restore owned rules")
				}
			} else {
				if f.rules[oldID] != nil {
					t.Fatal("old managed rule retained")
				}
				macs, _, err := u.managedAccess(t.Context())
				if err != nil || len(macs) != 1 || macs[0] != "AA:BB:CC:DD:EE:FF" {
					t.Fatalf("readback: %v %v", macs, err)
				}
			}
		})
	}
}
func TestAccessRuleNameCollisionDoesNotWrite(t *testing.T) {
	id := accessRuleID("AA:BB:CC:DD:EE:FF")
	f := &accessFixture{rules: map[string]any{"lan": map[string]any{".type": "zone", "name": "lan"}, id: map[string]any{"target": "ACCEPT"}}}
	u := NewU60Local(true)
	u.local.Runner = f
	if err := u.setManagedAccess(t.Context(), map[string]string{"BlackMacList": "AA:BB:CC:DD:EE:FF"}); err == nil || f.mutations != 0 {
		t.Fatal("changed another owner's rule")
	}
}

type failingAPN struct {
	calls    []string
	selected string
}

func (f *failingAPN) Call(_ context.Context, _, method string, p map[string]any) (map[string]any, error) {
	f.calls = append(f.calls, method)
	switch method {
	case "get_enabled_manu_apn_id":
		return map[string]any{"profileId": f.selected}, nil
	case "enable_manu_apn_id":
		f.selected = textValue(p["profileId"])
	case "set_apn_mode":
		return nil, errors.New("fixture mode failure")
	}
	return map[string]any{}, nil
}
func TestAPNModeFailureRestoresSelection(t *testing.T) {
	f := &failingAPN{selected: "old"}
	if err := SelectAPN(t.Context(), f, "manual", "new"); err == nil || f.selected != "old" {
		t.Fatalf("selection=%q err=%v", f.selected, err)
	}
}
func TestBandListUsesNativeHexAndNRRequiresSCS(t *testing.T) {
	u, f := localFixture(true)
	_, err := u.GoFormCommandResult(t.Context(), map[string]string{"goformId": "LTE_BAND_LOCK", "lte_band_lock": "1,3,41"})
	if err != nil {
		t.Fatal(err)
	}
	p := f.calls[len(f.calls)-1].params
	if p["lte_band_mask"] != "0x10000000005" {
		t.Fatalf("invalid mask %v", p)
	}
	n := len(f.calls)
	_, err = u.GoFormCommandResult(t.Context(), map[string]string{"goformId": "CELL_LOCK", "rat": "1", "pci": "20", "earfcn": "600000"})
	if !errors.Is(err, ErrInvalidArgument) || len(f.calls) != n {
		t.Fatal("incomplete NR lock executed")
	}
}
func TestSMSReadDoesNotRequireWriteAccess(t *testing.T) {
	u, f := localFixture(false)
	_, err := u.GoFormCommandResult(t.Context(), map[string]string{"goformId": "SEND_SMS", "Number": "10000", "MessageText": "fixture"})
	if !errors.Is(err, ErrWriteDisabled) {
		t.Fatal(err)
	}
	_, err = u.GoFormCommandResult(t.Context(), map[string]string{"goformId": "zwrt_wms", "func": "zte_libwms_get_sms_data", "page": "0", "data_per_page": "20"})
	if errors.Is(err, ErrWriteDisabled) || len(f.calls) != 1 {
		t.Fatalf("read rejected: %v", err)
	}
	if f.calls[0].params["data_per_page"] != float64(20) {
		t.Fatal("numeric metadata was stringified")
	}
	u.EnableWrites(true)
	_, err = u.GoFormCommandResult(t.Context(), map[string]string{"goformId": "SEND_SMS", "Number": "10000", "MessageBody": "4f60597d"})
	if err != nil {
		t.Fatal(err)
	}
	if f.calls[len(f.calls)-1].params["message_body"] != "你好" {
		t.Fatal("UTF-16 SMS not decoded")
	}
	if strings.Contains(textValue(f.calls[len(f.calls)-1].params), "MessageBody") {
		t.Fatal("legacy payload reached native RPC")
	}
}

func TestPerformanceControlsReadBackFromIsolatedSysfs(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "sys/devices/system/cpu/cpufreq/policy0")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "scaling_governor"), []byte("schedutil"), 0600)
	_ = os.WriteFile(filepath.Join(dir, "scaling_available_governors"), []byte("schedutil performance"), 0600)
	u, _ := localFixture(true)
	u.metrics.Root = root
	if err := u.setPerformance("1"); err != nil {
		t.Fatal(err)
	}
	if u.systemSnapshot([]string{"performance_mode"})["performance_mode"] != "1" {
		t.Fatal("setting did not take effect")
	}
	if err := u.setPerformance("0"); err != nil {
		t.Fatal(err)
	}
	if u.systemSnapshot([]string{"performance_mode"})["performance_mode"] != "0" {
		t.Fatal("normal mode not restored")
	}
}
