// SPDX-License-Identifier: AGPL-3.0-or-later

package device

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxZTEResponse = 2 << 20

type ZTEHTTP struct {
	base   *url.URL
	client *http.Client
}

func NewZTEHTTP(baseURL string, client *http.Client) (*ZTEHTTP, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse device URL: %w", err)
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return nil, errors.New("device URL must use HTTP or HTTPS")
	}
	if base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, errors.New("device URL must be an origin without credentials, query, or fragment")
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	base.Path = strings.TrimSuffix(base.Path, "/")
	return &ZTEHTTP{base: base, client: client}, nil
}

func (z *ZTEHTTP) Status(ctx context.Context) (Status, error) {
	values, err := z.get(ctx, []string{"modem_main_state", "network_type", "signalbar", "simcard_roam", "imei", "wa_inner_version"})
	if err != nil {
		return Status{}, err
	}
	signal, _ := strconv.Atoi(first(values, "signalbar", "signal"))
	state := strings.ToLower(values["modem_main_state"])
	return Status{
		Model:        first(values, "wa_inner_version", "model"),
		Manufacturer: "ZTE",
		IMEI:         values["imei"],
		Signal:       signal,
		NetworkType:  values["network_type"],
		Connected:    state == "modem_connected" || state == "connected" || state == "1",
		SIMReady:     values["simcard_roam"] != "-1" && state != "modem_sim_undetected",
	}, nil
}

func (z *ZTEHTTP) GoFormGet(ctx context.Context, key string) (string, error) {
	if !validGoFormName(key) {
		return "", ErrUnsupported
	}
	values, err := z.get(ctx, []string{key})
	if err != nil {
		return "", err
	}
	value, ok := values[key]
	if !ok {
		return "", ErrUnsupported
	}
	return value, nil
}

func (z *ZTEHTTP) GoFormSet(ctx context.Context, key, value string) error {
	if !validGoFormName(key) {
		return ErrUnsupported
	}
	return z.GoFormCommand(ctx, map[string]string{"goformId": key, key: value})
}

func (z *ZTEHTTP) GoFormGetMany(ctx context.Context, keys []string) (map[string]string, error) {
	return z.get(ctx, keys)
}

func (z *ZTEHTTP) GoFormCommand(ctx context.Context, values map[string]string) error {
	response, err := z.GoFormCommandResult(ctx, values)
	if err != nil {
		return err
	}
	result := strings.ToLower(fmt.Sprint(response["result"]))
	if result != "success" && result != "0" && result != "true" {
		return fmt.Errorf("ZTE rejected goform operation: %v", response["result"])
	}
	return nil
}

func (z *ZTEHTTP) GoFormCommandResult(ctx context.Context, values map[string]string) (map[string]any, error) {
	form := url.Values{"isTest": {"false"}}
	for key, value := range values {
		if !validGoFormName(key) || len(value) > 4096 {
			return nil, ErrUnsupported
		}
		form.Set(key, value)
	}
	if form.Get("goformId") == "" {
		return nil, errors.New("goformId is required")
	}
	requestURL := z.resolve("/goform/goform_set_cmd_process")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var response map[string]any
	if err := z.doJSON(req, &response); err != nil {
		return nil, err
	}
	return response, nil
}

func (z *ZTEHTTP) get(ctx context.Context, keys []string) (map[string]string, error) {
	for _, key := range keys {
		if !validGoFormName(key) {
			return nil, ErrUnsupported
		}
	}
	query := url.Values{"isTest": {"false"}, "cmd": {strings.Join(keys, ",")}, "multi_data": {"1"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, z.resolve("/goform/goform_get_cmd_process")+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := z.doJSON(req, &raw); err != nil {
		return nil, err
	}
	result := make(map[string]string, len(raw))
	for key, value := range raw {
		if value != nil {
			result[key] = fmt.Sprint(value)
		}
	}
	return result, nil
}

func (z *ZTEHTTP) doJSON(req *http.Request, target any) error {
	resp, err := z.client.Do(req)
	if err != nil {
		return fmt.Errorf("ZTE request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("ZTE response status %d", resp.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxZTEResponse))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode ZTE response: %w", err)
	}
	return nil
}

func (z *ZTEHTTP) resolve(path string) string {
	copy := *z.base
	copy.Path = strings.TrimSuffix(z.base.Path, "/") + path
	return copy.String()
}

func validGoFormName(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if !(r == '_' || r == '-' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func first(values map[string]string, keys ...string) string {
	for _, key := range keys {
		if values[key] != "" {
			return values[key]
		}
	}
	return "U60 Pro"
}

func IsPrivateDeviceHost(hostport string) bool {
	host := hostport
	if parsed, _, err := net.SplitHostPort(hostport); err == nil {
		host = parsed
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast())
}
