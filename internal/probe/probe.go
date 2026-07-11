// SPDX-License-Identifier: AGPL-3.0-or-later

package probe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const maxResponse = 4 << 20

var DefaultPaths = []string{
	"/healthz",
	"/api/need_token",
	"/api/baseDeviceInfo",
	"/api/connInfo",
	"/api/device_id",
	"/api/device_uuid",
	"/api/version_info",
	"/api/SELinux",
	"/api/usb_status",
	"/api/adb/status",
	"/api/adb_alive",
	"/api/adb_wifi_setting",
	"/api/ttyd/status",
	"/api/hasTTYD?port=1146",
	"/api/time/status",
	"/api/get_cookie",
	"/api/get_data_limit",
	"/api/get_log_status",
	"/api/get_res_server",
	"/api/get_theme",
	"/api/is_weak_token",
	"/api/cellularUsage",
	"/api/power_status_forward_enabled",
	"/api/list_tasks",
	"/api/sms_forward_enabled",
	"/api/sms_forward_method",
	"/api/sms_forward_blacklist",
	"/api/sms_forward_curl",
	"/api/sms_forward_dingtalk",
	"/api/sms_forward_mail",
	"/api/plugins_store",
	"/api/plugin/list",
	"/api/list_uploads",
	"/api/check_update",
	"/api/download_update_status",
	"/api/auto_update/config",
	"/api/volte_status",
	"/api/vonr_status",
}

type Result struct {
	CapturedAt string            `json:"captured_at"`
	Base       string            `json:"base"`
	Entries    []Entry           `json:"entries"`
	Errors     map[string]string `json:"errors,omitempty"`
}

type Entry struct {
	Path        string `json:"path"`
	Status      int    `json:"status"`
	ContentType string `json:"content_type,omitempty"`
	Body        any    `json:"body,omitempty"`
}

type Client struct {
	base  *url.URL
	token string
	http  *http.Client
	now   func() time.Time
}

func New(baseURL, token string, client *http.Client) (*Client, error) {
	base, err := url.Parse(baseURL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return nil, errors.New("probe base must be an HTTP(S) origin")
	}
	if base.User != nil || base.RawQuery != "" || base.Fragment != "" || strings.Trim(base.Path, "/") != "" {
		return nil, errors.New("probe base must not contain credentials, path, query, or fragment")
	}
	if !privateHost(base.Hostname()) {
		return nil, errors.New("probe base must use a private, loopback, or link-local address")
	}
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	return &Client{base: base, token: token, http: client, now: time.Now}, nil
}

func (c *Client) Capture(ctx context.Context, paths []string) Result {
	if len(paths) == 0 {
		paths = DefaultPaths
	}
	paths = append([]string(nil), paths...)
	sort.Strings(paths)
	result := Result{CapturedAt: c.now().UTC().Format(time.RFC3339), Base: c.base.Scheme + "://" + c.base.Host, Errors: map[string]string{}}
	for _, path := range paths {
		entry, err := c.capture(ctx, path)
		if err != nil {
			result.Errors[path] = err.Error()
			continue
		}
		result.Entries = append(result.Entries, entry)
	}
	if len(result.Errors) == 0 {
		result.Errors = nil
	}
	return result
}

func (c *Client) capture(ctx context.Context, path string) (Entry, error) {
	relative, err := url.Parse(path)
	if err != nil || !strings.HasPrefix(relative.Path, "/") || relative.IsAbs() || relative.Host != "" {
		return Entry{}, errors.New("invalid probe path")
	}
	if !safePath(relative.Path) {
		return Entry{}, errors.New("path is not in the read-only probe allowlist")
	}
	target := *c.base
	target.Path, target.RawQuery = relative.Path, relative.RawQuery
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return Entry{}, err
	}
	if c.token != "" {
		req.Header.Set("X-Device-Token", c.token)
		// The open replacement accepts X-Device-Token. Historical UFI-TOOLS
		// builds compare the raw Authorization value after hashing it locally.
		req.Header.Set("Authorization", c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return Entry{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil {
		return Entry{}, err
	}
	if len(body) > maxResponse {
		return Entry{}, errors.New("response exceeded probe limit")
	}
	entry := Entry{Path: path, Status: resp.StatusCode, ContentType: resp.Header.Get("Content-Type")}
	var decoded any
	if len(body) != 0 && json.Unmarshal(body, &decoded) == nil {
		entry.Body = Redact(decoded)
	} else if len(body) != 0 {
		entry.Body = map[string]any{"text_length": len(body)}
	}
	return entry, nil
}

func safePath(path string) bool {
	if path == "/healthz" {
		return true
	}
	for _, allowed := range DefaultPaths {
		parsed, _ := url.Parse(allowed)
		if parsed.Path == path {
			return true
		}
	}
	return false
}

func Redact(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			if sensitiveKey(key) {
				result[key] = redactedValue(item)
			} else {
				result[key] = Redact(item)
			}
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for i, item := range typed {
			result[i] = Redact(item)
		}
		return result
	default:
		return value
	}
}

func sensitiveKey(key string) bool {
	key = strings.ToLower(strings.ReplaceAll(key, "-", "_"))
	for _, marker := range []string{"token", "password", "passwd", "pwd", "secret", "cookie", "authorization", "imei", "imsi", "iccid", "msisdn", "phone", "uuid", "device_id", "mac_address"} {
		if key == marker || strings.HasSuffix(key, "_"+marker) || strings.HasPrefix(key, marker+"_") {
			return true
		}
	}
	return false
}

func redactedValue(value any) any {
	switch value.(type) {
	case nil:
		return nil
	case bool:
		return false
	case float64, float32, int, int64, json.Number:
		return 0
	default:
		return "<redacted>"
	}
}

func privateHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast())
}

func Encode(w io.Writer, result Result) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		return fmt.Errorf("encode probe result: %w", err)
	}
	return nil
}
