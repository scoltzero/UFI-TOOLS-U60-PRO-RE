// SPDX-License-Identifier: AGPL-3.0-or-later
package device

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestU60UBUSStatus(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var c []struct {
			Params []any `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&c)
		v := map[string]any{}
		switch c[0].Params[2].(string) {
		case "get_device_info":
			v = map[string]any{"wa_inner_version": "U60", "modem_imei": "secret", "device_uptime": 12}
		case "nwinfo_get_netinfo":
			v = map[string]any{"network_type": "NR5G-SA", "signalbar": 4, "simcard_roam": 0}
		case "get_sim_info_before":
			v = map[string]any{"modem_main_state": "modem_connected"}
		}
		_ = json.NewEncoder(w).Encode([]any{map[string]any{"result": []any{0, v}}})
	}))
	defer s.Close()
	d, e := NewU60UBUS(s.URL, "", s.Client())
	if e != nil {
		t.Fatal(e)
	}
	v, e := d.Status(t.Context())
	if e != nil || v.Model != "U60" || v.NetworkType != "NR5G-SA" || v.Signal != 4 || !v.Connected {
		t.Fatalf("status=%#v err=%v", v, e)
	}
}
