// SPDX-License-Identifier: AGPL-3.0-or-later

package laninstall

import (
	"bytes"
	"context"
	"crypto/sha256"
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
	defaultSession     = "00000000000000000000000000000000"
	webProcedureSource = "zwrt_web"
	maxBackupSize      = 128 << 20
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

func (c *Client) Login(ctx context.Context, password string) error {
	if password == "" {
		return errors.New("router password is empty")
	}
	info, err := c.call(ctx, defaultSession, "zwrt_web", "web_login_info", map[string]string{})
	if err != nil {
		return fmt.Errorf("read router login salt: %w", err)
	}
	salt, _ := info["zte_web_sault"].(string)
	if salt == "" {
		return errors.New("router login salt is missing")
	}
	first := sha256UpperHex(password)
	digest := sha256UpperHex(first + salt)
	loggedIn, err := c.call(ctx, defaultSession, "zwrt_web", "web_login", map[string]string{"password": digest})
	if err != nil {
		return fmt.Errorf("router login: %w", err)
	}
	if !zeroResult(loggedIn["result"]) {
		return errors.New("router rejected the password")
	}
	session, _ := loggedIn["ubus_rpc_session"].(string)
	if len(session) != 32 || strings.Trim(session, "0123456789abcdefABCDEF") != "" {
		return errors.New("router login returned an invalid session")
	}
	c.session = session
	return nil
}

func sha256UpperHex(value string) string {
	digest := sha256.Sum256([]byte(value))
	return strings.ToUpper(fmt.Sprintf("%x", digest))
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
	request.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	request.Header.Set("Cache-Control", "no-cache")
	request.Header.Set("Pragma", "no-cache")
	request.Header.Set("Upgrade-Insecure-Requests", "1")
	request.Header.Set("User-Agent", "Mozilla/5.0 UFI-TOOLS-U60-PRO-RE")
	if c.session != defaultSession {
		request.AddCookie(&http.Cookie{Name: "webtoken", Value: c.session})
	}
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
	object, err := c.call(ctx, c.session, "zwrt_mc.device.manager", procedure, map[string]string{"procType": webProcedureSource})
	if err != nil {
		return err
	}
	value, _ := object["result"].(string)
	if !strings.EqualFold(value, "success") {
		return errors.New("UBUS operation failed")
	}
	return nil
}

func (c *Client) call(ctx context.Context, session, object, procedure string, arguments any) (map[string]any, error) {
	payload := []map[string]any{{
		"jsonrpc": "2.0", "id": 1, "method": "call",
		"params": []any{session, object, procedure, arguments},
	}}
	encoded, _ := json.Marshal(payload)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.resolve("/ubus/?t="+fmt.Sprint(time.Now().UnixMilli())), bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Z-Mode", "0")
	request.Header.Set("Z-Tag", "")
	request.Header.Set("Origin", c.base.Scheme+"://"+c.base.Host)
	request.Header.Set("Referer", c.base.Scheme+"://"+c.base.Host+"/")
	request.Header.Set("X-Requested-With", "XMLHttpRequest")
	if session != defaultSession {
		request.AddCookie(&http.Cookie{Name: "webtoken", Value: session})
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("UBUS returned %d", response.StatusCode)
	}
	var result []struct {
		Error  any   `json:"error"`
		Result []any `json:"result"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(&result); err != nil || len(result) != 1 || result[0].Error != nil {
		return nil, errors.New("invalid UBUS response")
	}
	if len(result[0].Result) < 2 {
		return nil, errors.New("UBUS response is incomplete")
	}
	code, ok := result[0].Result[0].(float64)
	objectResult, objectOK := result[0].Result[1].(map[string]any)
	if !ok || code != 0 || !objectOK {
		return nil, errors.New("UBUS call failed")
	}
	return objectResult, nil
}

func zeroResult(value any) bool {
	switch typed := value.(type) {
	case string:
		return typed == "0"
	case float64:
		return typed == 0
	default:
		return false
	}
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
