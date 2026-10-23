// SPDX-License-Identifier: AGPL-3.0-or-later
package device

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/u60"
)

const anonymousUBUSSession = "00000000000000000000000000000000"

type U60UBUS struct {
	base        *url.URL
	session     string
	client      *http.Client
	local       *u60.UBUS
	metrics     *u60.Telemetry
	allowWrites bool
	cacheMu     sync.Mutex
	cacheAt     time.Time
	cache       map[string]any
}

func NewU60UBUS(baseURL, session string, client *http.Client) (*U60UBUS, error) {
	base, err := url.Parse(baseURL)
	if err != nil || base.Host == "" || base.User != nil || (base.Scheme != "http" && base.Scheme != "https") {
		return nil, errors.New("invalid U60 UBUS origin")
	}
	if session == "" {
		session = anonymousUBUSSession
	}
	if len(session) != 32 || strings.Trim(session, "0123456789abcdefABCDEF") != "" {
		return nil, errors.New("U60 UBUS session must be 32 hexadecimal characters")
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	base.Path, base.RawQuery, base.Fragment = strings.TrimSuffix(base.Path, "/"), "", ""
	return &U60UBUS{base: base, session: session, client: client}, nil
}

func (u *U60UBUS) Status(ctx context.Context) (Status, error) {
	v, err := u.GoFormGetMany(ctx, []string{"model", "device_model", "modem_main_state", "network_type", "signalbar", "simcard_roam", "imei", "device_uptime", "current_wan_status", "sim_states"})
	if err != nil {
		return Status{}, err
	}
	signal, _ := strconv.Atoi(v["signalbar"])
	uptime, _ := strconv.ParseInt(v["device_uptime"], 10, 64)
	state := strings.ToLower(v["modem_main_state"])
	wan := strings.ToLower(v["current_wan_status"])
	sim := strings.ToLower(v["sim_states"])
	connected := state == "modem_connected" || state == "connected" || state == "1" || wan == "connected" || wan == "ipv4_ipv6_connected" || wan == "ipv4_connected" || wan == "ipv6_connected"
	simReady := sim == "sim ready" || sim == "ready"
	if sim == "" && state != "modem_sim_undetected" {
		roam, present := v["simcard_roam"]
		simReady = present && roam != "" && roam != "-1"
	}
	model := first(v, "model", "device_model")
	if model == "" {
		model = "U60 Pro"
	}
	return Status{Model: model, Manufacturer: "ZTE", IMEI: v["imei"], Signal: signal, NetworkType: v["network_type"], Connected: connected, SIMReady: simReady, Uptime: uptime}, nil
}
func (u *U60UBUS) GoFormGet(ctx context.Context, key string) (string, error) {
	v, e := u.GoFormGetMany(ctx, []string{key})
	if e != nil {
		return "", e
	}
	x, ok := v[key]
	if !ok {
		return "", ErrUnsupported
	}
	return x, nil
}
func (u *U60UBUS) GoFormGetMany(ctx context.Context, keys []string) (map[string]string, error) {
	values, err := u.Snapshot(ctx, keys)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(keys))
	for k, x := range values {
		if x == nil {
			continue
		}
		switch x.(type) {
		case map[string]any, []any, []map[string]any:
			b, _ := json.Marshal(x)
			out[k] = string(b)
		default:
			out[k] = fmt.Sprint(x)
		}
	}
	return out, nil
}
func (u *U60UBUS) Call(ctx context.Context, module, procedure string, params map[string]any) (map[string]any, error) {
	if !u.allowWrites && !readOnlyUBUS(procedure) {
		return nil, ErrWriteDisabled
	}
	if !readOnlyUBUS(procedure) {
		defer u.invalidateStatus()
	}
	return u.call(ctx, module, procedure, params)
}
func (u *U60UBUS) call(ctx context.Context, module, procedure string, params map[string]any) (map[string]any, error) {
	if u.local != nil {
		value := u.local.Call(ctx, module, procedure, params)
		if u60.HasError(value) {
			return nil, fmt.Errorf("%s.%s: %v", module, procedure, value["error"])
		}
		return value, nil
	}
	p := []map[string]any{{"jsonrpc": "2.0", "id": 1, "method": "call", "params": []any{u.session, module, procedure, params}}}
	encoded, _ := json.Marshal(p)
	target := *u.base
	target.Path = strings.TrimSuffix(u.base.Path, "/") + "/ubus/"
	target.RawQuery = "t=" + strconv.FormatInt(time.Now().UnixMilli(), 10)
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(encoded))
	if e != nil {
		return nil, e
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Z-Mode", "0")
	resp, e := u.client.Do(req)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var result []struct {
		Error  any               `json:"error"`
		Result []json.RawMessage `json:"result"`
	}
	if e = json.NewDecoder(io.LimitReader(resp.Body, maxZTEResponse)).Decode(&result); e != nil || len(result) != 1 || result[0].Error != nil || len(result[0].Result) < 2 {
		return nil, errors.New("invalid UBUS response")
	}
	var code int
	var values map[string]any
	if json.Unmarshal(result[0].Result[0], &code) != nil || code != 0 || json.Unmarshal(result[0].Result[1], &values) != nil {
		return nil, errors.New("UBUS call failed")
	}
	if u60.HasError(values) {
		return nil, errors.New("UBUS operation was rejected")
	}
	return values, nil
}
