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
	"time"
)

const anonymousUBUSSession = "00000000000000000000000000000000"

type U60UBUS struct {
	base    *url.URL
	session string
	client  *http.Client
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
	v, err := u.GoFormGetMany(ctx, []string{"wa_inner_version", "modem_main_state", "network_type", "signalbar", "simcard_roam", "imei", "device_uptime", "current_wan_status", "sim_states"})
	if err != nil {
		return Status{}, err
	}
	signal, _ := strconv.Atoi(v["signalbar"])
	uptime, _ := strconv.ParseInt(v["device_uptime"], 10, 64)
	state := strings.ToLower(v["modem_main_state"])
	wan := strings.ToLower(v["current_wan_status"])
	sim := strings.ToLower(v["sim_states"])
	connected := state == "modem_connected" || state == "connected" || state == "1" || strings.Contains(wan, "connected")
	simReady := (strings.Contains(sim, "ready") || v["simcard_roam"] != "-1") && state != "modem_sim_undetected"
	return Status{Model: first(v, "wa_inner_version"), Manufacturer: "ZTE", IMEI: v["imei"], Signal: signal, NetworkType: v["network_type"], Connected: connected, SIMReady: simReady, Uptime: uptime}, nil
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
func (u *U60UBUS) GoFormSet(context.Context, string, string) error { return ErrUnsupported }
func (u *U60UBUS) GoFormGetMany(ctx context.Context, keys []string) (map[string]string, error) {
	merged := map[string]any{}
	for _, c := range [][2]string{{"zwrt_mc.device.manager", "get_device_info"}, {"zte_nwinfo_api", "nwinfo_get_netinfo"}, {"zwrt_zte_mdm.api", "get_sim_info_before"}, {"zwrt_router.api", "router_get_status_no_auth"}} {
		v, e := u.call(ctx, c[0], c[1], map[string]any{})
		if e != nil {
			return nil, fmt.Errorf("U60 %s.%s: %w", c[0], c[1], e)
		}
		for k, x := range v {
			merged[k] = x
		}
	}
	aliases := map[string]string{"imei": "modem_imei", "hw_version": "wa_version", "signal": "signalbar", "model": "wa_inner_version"}
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		s := k
		if aliases[k] != "" {
			s = aliases[k]
		}
		if x, ok := merged[s]; ok && x != nil {
			out[k] = fmt.Sprint(x)
		}
	}
	return out, nil
}
func (u *U60UBUS) Call(ctx context.Context, module, procedure string, params map[string]any) (map[string]any, error) {
	return u.call(ctx, module, procedure, params)
}
func (u *U60UBUS) call(ctx context.Context, module, procedure string, params map[string]any) (map[string]any, error) {
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
	return values, nil
}
