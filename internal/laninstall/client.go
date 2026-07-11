// SPDX-License-Identifier: AGPL-3.0-or-later

package laninstall

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultSession = "00000000000000000000000000000000"
	maxBackupSize  = 128 << 20
)

type Client struct {
	base    *url.URL
	session string
	http    *http.Client
}

func NewClient(baseURL, session string, client *http.Client) (*Client, error) {
	base, err := url.Parse(baseURL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return nil, errors.New("LAN installer base must be an HTTP(S) origin")
	}
	if base.User != nil || base.RawQuery != "" || base.Fragment != "" || strings.Trim(base.Path, "/") != "" {
		return nil, errors.New("LAN installer base must not contain credentials, path, query, or fragment")
	}
	if !privateHost(base.Hostname()) {
		return nil, errors.New("LAN installer requires a private, loopback, or link-local target")
	}
	if session == "" {
		session = defaultSession
	}
	if len(session) != 32 || strings.Trim(session, "0123456789abcdefABCDEF") != "" {
		return nil, errors.New("UBUS session must be a 32-character hexadecimal value")
	}
	if client == nil {
		client = &http.Client{Timeout: 45 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}}
	}
	return &Client{base: base, session: session, http: client}, nil
}

func (c *Client) Backup(ctx context.Context) ([]byte, error) {
	if err := c.rpc(ctx, "device_backup_proc"); err != nil {
		return nil, fmt.Errorf("prepare vendor backup: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.resolve("/backup/back_parameter"), nil)
	if err != nil {
		return nil, err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download vendor backup: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download vendor backup returned %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxBackupSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || len(data) > maxBackupSize {
		return nil, errors.New("vendor backup is empty or too large")
	}
	return data, nil
}

func (c *Client) Upload(ctx context.Context, backup []byte) error {
	if len(backup) == 0 || len(backup) > maxBackupSize {
		return errors.New("restore container is empty or too large")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("filename", "/tmp/back_parameter"); err != nil {
		return err
	}
	part, err := writer.CreateFormFile("filedata", "backup_restore.bin")
	if err != nil {
		return err
	}
	if _, err := part.Write(backup); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.resolve("/cgi-bin/cgi-upload"), &body)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Origin", c.base.Scheme+"://"+c.base.Host)
	request.Header.Set("Referer", c.base.Scheme+"://"+c.base.Host+"/")
	request.Header.Set("X-Requested-With", "XMLHttpRequest")
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("upload restore container: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("restore upload returned %d", response.StatusCode)
	}
	text := strings.ToLower(string(data))
	if !strings.Contains(text, "checksum") || !strings.Contains(text, "sha256sum") {
		return errors.New("restore upload did not return checksum confirmation")
	}
	return nil
}

func (c *Client) Restore(ctx context.Context) error {
	return c.rpc(ctx, "device_restore_proc")
}

func (c *Client) rpc(ctx context.Context, procedure string) error {
	payload := []map[string]any{{
		"jsonrpc": "2.0", "id": 1, "method": "call",
		"params": []any{c.session, "zwrt_mc.device.manager", procedure, map[string]string{"procType": "web"}},
	}}
	encoded, _ := json.Marshal(payload)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.resolve("/ubus/?t="+fmt.Sprint(time.Now().UnixMilli())), bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Z-Mode", "0")
	request.Header.Set("Z-Tag", "")
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("UBUS returned %d", response.StatusCode)
	}
	var result []struct {
		Error  any   `json:"error"`
		Result []any `json:"result"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(&result); err != nil || len(result) != 1 || result[0].Error != nil {
		return errors.New("invalid UBUS response")
	}
	if !rpcSuccess(result[0].Result) {
		return errors.New("UBUS operation failed")
	}
	return nil
}

func rpcSuccess(result []any) bool {
	if len(result) < 2 {
		return false
	}
	code, ok := result[0].(float64)
	if !ok || code != 0 {
		return false
	}
	object, ok := result[1].(map[string]any)
	if !ok {
		return false
	}
	value, _ := object["result"].(string)
	return strings.EqualFold(value, "success")
}

func (c *Client) resolve(relative string) string {
	copy := *c.base
	parts := strings.SplitN(relative, "?", 2)
	copy.Path = parts[0]
	if len(parts) == 2 {
		copy.RawQuery = parts[1]
	}
	return copy.String()
}

func privateHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast())
}
