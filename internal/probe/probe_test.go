// SPDX-License-Identifier: AGPL-3.0-or-later

package probe

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCaptureRedactsSensitiveFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Device-Token") != "probe-token" {
			t.Fatal("missing probe token")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "U60 Pro", "device_token": "secret", "nested": map[string]any{"imei": "123", "signal": 4},
		})
	}))
	defer server.Close()
	client, err := New(server.URL, "probe-token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.now = func() time.Time { return time.Unix(1, 0) }
	result := client.Capture(context.Background(), []string{"/api/baseDeviceInfo"})
	if len(result.Entries) != 1 || result.Entries[0].Status != 200 {
		t.Fatalf("unexpected result: %#v", result)
	}
	body := result.Entries[0].Body.(map[string]any)
	if body["device_token"] != "<redacted>" || body["model"] != "U60 Pro" {
		t.Fatalf("redaction failed: %#v", body)
	}
	nested := body["nested"].(map[string]any)
	if nested["imei"] != "<redacted>" || nested["signal"] != float64(4) {
		t.Fatalf("nested redaction failed: %#v", nested)
	}
}

func TestRejectsPublicAndMutatingTargets(t *testing.T) {
	if _, err := New("https://example.com", "", nil); err == nil {
		t.Fatal("expected public target rejection")
	}
	client, err := New("http://127.0.0.1:2333", "", &http.Client{})
	if err != nil {
		t.Fatal(err)
	}
	result := client.Capture(context.Background(), []string{"/api/disable_fota"})
	if !strings.Contains(result.Errors["/api/disable_fota"], "allowlist") {
		t.Fatalf("expected allowlist rejection: %#v", result.Errors)
	}
}
