// SPDX-License-Identifier: AGPL-3.0-or-later

package device

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestZTEHTTPGetSetAndStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/goform/goform_get_cmd_process":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"network_type": "NR5G", "signalbar": 4, "modem_main_state": "modem_connected",
				"simcard_roam": "0", "imei": "redacted", "wa_inner_version": "U60 Pro",
			})
		case "/goform/goform_set_cmd_process":
			if r.Method != http.MethodPost || r.FormValue("goformId") != "APN_config0" {
				t.Fatalf("unexpected set request: %s %#v", r.Method, r.Form)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"result": "success"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	device, err := NewZTEHTTP(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	status, err := device.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.Connected || status.NetworkType != "NR5G" || status.Signal != 4 {
		t.Fatalf("unexpected status: %#v", status)
	}
	if err := device.GoFormSet(context.Background(), "APN_config0", "internet"); err != nil {
		t.Fatal(err)
	}
}

func TestZTEHTTPRejectsUnsafeInputs(t *testing.T) {
	if _, err := NewZTEHTTP("file:///etc/passwd", nil); err == nil {
		t.Fatal("expected scheme rejection")
	}
	device, err := NewZTEHTTP("http://127.0.0.1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := device.GoFormGet(context.Background(), "bad,key"); err == nil {
		t.Fatal("expected key validation failure")
	}
}
