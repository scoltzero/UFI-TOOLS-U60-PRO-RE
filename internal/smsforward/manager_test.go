// SPDX-License-Identifier: AGPL-3.0-or-later

package smsforward

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/device"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/store"
)

type smsDevice struct {
	messages []map[string]any
	commands []map[string]string
}

func (d *smsDevice) Status(context.Context) (device.Status, error) { return device.Status{}, nil }
func (d *smsDevice) GoFormGet(context.Context, string) (string, error) {
	return "", device.ErrUnsupported
}
func (d *smsDevice) GoFormSet(context.Context, string, string) error { return nil }
func (d *smsDevice) GoFormCommand(ctx context.Context, values map[string]string) error {
	_, err := d.GoFormCommandResult(ctx, values)
	return err
}
func (d *smsDevice) GoFormCommandResult(_ context.Context, values map[string]string) (map[string]any, error) {
	d.commands = append(d.commands, values)
	if values["func"] == "zte_libwms_get_sms_data" {
		return map[string]any{"messages": d.messages}, nil
	}
	return map[string]any{"result": "success"}, nil
}

func TestTickForwardsNewHTTPMessageOnce(t *testing.T) {
	var received string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		received = string(data)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	state, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	_ = state.Set(context.Background(), "kv", "sms_forward_enabled", "true")
	_ = state.Set(context.Background(), "kv", "sms_forward_method", "CURL")
	_ = state.Set(context.Background(), "kv", "sms_forward_curl", `{"url":"`+server.URL+`","body":"{{sms-from}}:{{sms-body}}"}`)
	target := &smsDevice{messages: []map[string]any{{"id": 7, "number": "10086", "content": "hello", "date": "now"}}}
	manager := New(state, target, 0, server.Client(), nil)
	if err := manager.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(received, "10086:hello") {
		t.Fatalf("unexpected forwarded body: %q", received)
	}
	received = ""
	if err := manager.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if received != "" {
		t.Fatal("message forwarded more than once")
	}
}

func TestSMSMethodUsesZTECommand(t *testing.T) {
	state, _ := store.Open(context.Background(), filepath.Join(t.TempDir(), "state.db"))
	defer state.Close()
	_ = state.Set(context.Background(), "kv", "sms_forward_enabled", "true")
	_ = state.Set(context.Background(), "kv", "sms_forward_method", "SMS")
	_ = state.Set(context.Background(), "kv", "sms_forward_sms", `{"number":"13800138000"}`)
	target := &smsDevice{messages: []map[string]any{{"id": 1, "number": "10010", "content": "notice"}}}
	manager := New(state, target, 0, nil, nil)
	if err := manager.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	last := target.commands[len(target.commands)-1]
	if last["func"] != "zte_libwms_send_sms" || last["number"] != "13800138000" {
		t.Fatalf("unexpected command: %#v", last)
	}
}
