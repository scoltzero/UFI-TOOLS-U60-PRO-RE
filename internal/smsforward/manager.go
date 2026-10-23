// SPDX-License-Identifier: AGPL-3.0-or-later

package smsforward

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/device"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/store"
)

type Message struct {
	ID   int64  `json:"id"`
	From string `json:"from"`
	Body string `json:"body"`
	Date string `json:"date,omitempty"`
	Tag  string `json:"tag,omitempty"`
}

type Manager struct {
	store    *store.Store
	device   device.Device
	client   *http.Client
	logger   *slog.Logger
	interval time.Duration
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

func New(state *store.Store, target device.Device, interval time.Duration, client *http.Client, logger *slog.Logger) *Manager {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	clientCopy := *client
	clientCopy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{store: state, device: target, interval: interval, client: &clientCopy, logger: logger}
}

func (m *Manager) Start(parent context.Context) {
	if m.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	m.cancel = cancel
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(m.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := m.Tick(ctx); err != nil && !errors.Is(err, context.Canceled) {
					m.logger.Warn("SMS forwarding tick failed", "error", err)
				}
			}
		}
	}()
}

func (m *Manager) Stop() {
	if m.cancel != nil {
		m.cancel()
		m.wg.Wait()
		m.cancel = nil
	}
}

func (m *Manager) Tick(ctx context.Context) error {
	enabled, _ := m.setting("sms_forward_enabled")
	if !parseBool(enabled) {
		return nil
	}
	messages, err := m.list(ctx)
	if err != nil {
		return err
	}
	lastValue, _ := m.store.Get(ctx, "kv", "sms_forward_last_id")
	lastID, _ := strconv.ParseInt(strings.Trim(lastValue, `"`), 10, 64)
	sort.Slice(messages, func(i, j int) bool { return messages[i].ID < messages[j].ID })
	for _, message := range messages {
		if message.ID <= lastID {
			continue
		}
		if !m.blacklisted(message) {
			if err := m.forward(ctx, message); err != nil {
				return err
			}
		}
		lastID = message.ID
		if err := m.store.Set(ctx, "kv", "sms_forward_last_id", strconv.FormatInt(lastID, 10)); err != nil {
			return err
		}
	}
	return nil
}

// Forward sends one caller-supplied message through the configured forwarding
// method. It is used by the authenticated legacy test/forward endpoint.
func (m *Manager) Forward(ctx context.Context, message Message) error {
	return m.forward(ctx, message)
}

func (m *Manager) list(ctx context.Context) ([]Message, error) {
	response, err := device.CommandResult(ctx, m.device, map[string]string{
		"goformId": "zwrt_wms", "func": "zte_libwms_get_sms_data", "tags": "10",
		"page": "0", "data_per_page": "20", "mem_store": "1", "order_by": "order by id desc",
	})
	if err != nil {
		return nil, err
	}
	value := response["messages"]
	if value == nil {
		value = response["sms"]
	}
	if value == nil {
		value = response["data"]
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if text, ok := value.(string); ok {
		encoded = []byte(text)
	}
	var raw []map[string]any
	if err := json.Unmarshal(encoded, &raw); err != nil {
		return nil, fmt.Errorf("decode SMS list: %w", err)
	}
	result := make([]Message, 0, len(raw))
	for _, item := range raw {
		id, _ := strconv.ParseInt(toString(item["id"]), 10, 64)
		if id == 0 {
			id, _ = strconv.ParseInt(toString(item["sms_id"]), 10, 64)
		}
		if id == 0 {
			continue
		}
		result = append(result, Message{
			ID: id, From: first(item, "number", "address", "from"), Body: first(item, "content", "body", "message"),
			Date: first(item, "date", "time"), Tag: first(item, "tag"),
		})
	}
	return result, nil
}

func (m *Manager) forward(ctx context.Context, message Message) error {
	method, _ := m.setting("sms_forward_method")
	switch strings.ToUpper(settingString(method, "method", "sms_forward_method")) {
	case "CURL":
		return m.forwardHTTP(ctx, "sms_forward_curl", message, false)
	case "DINGTALK":
		return m.forwardHTTP(ctx, "sms_forward_dingtalk", message, true)
	case "SMS":
		config, _ := m.setting("sms_forward_sms")
		var target struct {
			Number string `json:"number"`
		}
		if json.Unmarshal([]byte(config), &target) != nil || target.Number == "" {
			return errors.New("SMS forwarding target is not configured")
		}
		return device.Command(ctx, m.device, map[string]string{
			"goformId": "zwrt_wms", "func": "zte_libwms_send_sms", "number": target.Number,
			"message_body": "来自 " + message.From + "\n" + message.Body, "encode_type": "UNICODE", "sms_time": "",
		})
	case "SMTP":
		return m.forwardSMTP(message)
	default:
		return fmt.Errorf("unknown SMS forwarding method %q", method)
	}
}

func (m *Manager) forwardSMTP(message Message) error {
	configText, _ := m.setting("sms_forward_mail")
	var config struct {
		Host        string `json:"host"`
		Port        int    `json:"port"`
		Username    string `json:"username"`
		Password    string `json:"password"`
		From        string `json:"from"`
		To          string `json:"to"`
		ImplicitTLS bool   `json:"implicit_tls"`
	}
	if json.Unmarshal([]byte(configText), &config) != nil || config.Host == "" || config.From == "" || config.To == "" {
		return errors.New("SMTP forwarding configuration is incomplete")
	}
	if strings.ContainsAny(config.Host, "\r\n/\\") || strings.ContainsAny(config.From+config.To, "\r\n") {
		return errors.New("SMTP configuration contains invalid characters")
	}
	if config.Port == 0 {
		if config.ImplicitTLS {
			config.Port = 465
		} else {
			config.Port = 587
		}
	}
	if config.Port < 1 || config.Port > 65535 {
		return errors.New("SMTP port is invalid")
	}
	address := net.JoinHostPort(config.Host, strconv.Itoa(config.Port))
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	var client *smtp.Client
	if config.ImplicitTLS {
		connection, err := tls.DialWithDialer(dialer, "tcp", address, &tls.Config{ServerName: config.Host, MinVersion: tls.VersionTLS12})
		if err != nil {
			return fmt.Errorf("SMTP TLS dial: %w", err)
		}
		created, err := smtp.NewClient(connection, config.Host)
		if err != nil {
			connection.Close()
			return err
		}
		client = created
	} else {
		connection, err := dialer.Dial("tcp", address)
		if err != nil {
			return fmt.Errorf("SMTP dial: %w", err)
		}
		created, err := smtp.NewClient(connection, config.Host)
		if err != nil {
			connection.Close()
			return err
		}
		client = created
		if ok, _ := client.Extension("STARTTLS"); !ok {
			client.Close()
			return errors.New("SMTP server does not offer STARTTLS")
		} else if err := client.StartTLS(&tls.Config{ServerName: config.Host, MinVersion: tls.VersionTLS12}); err != nil {
			client.Close()
			return err
		}
	}
	defer client.Close()
	if config.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", config.Username, config.Password, config.Host)); err != nil {
			return fmt.Errorf("SMTP authentication: %w", err)
		}
	}
	if err := client.Mail(config.From); err != nil {
		return err
	}
	recipients := splitList(config.To)
	if len(recipients) == 0 {
		return errors.New("SMTP recipient is empty")
	}
	for _, recipient := range recipients {
		if err := client.Rcpt(strings.TrimSpace(recipient)); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	subject := "UFI-TOOLS SMS from " + strings.ReplaceAll(message.From, "\r", "")
	body := "From: " + config.From + "\r\nTo: " + strings.Join(recipients, ", ") + "\r\nSubject: " + subject +
		"\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n" + message.Body + "\r\n"
	if _, err := io.WriteString(writer, body); err != nil {
		writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func (m *Manager) forwardHTTP(ctx context.Context, key string, message Message, dingTalk bool) error {
	configText, _ := m.setting(key)
	var config struct {
		URL     string            `json:"url"`
		Webhook string            `json:"webhook"`
		Method  string            `json:"method"`
		Headers map[string]string `json:"headers"`
		Body    string            `json:"body"`
	}
	if json.Unmarshal([]byte(configText), &config) != nil {
		return errors.New("HTTP forwarding configuration is invalid")
	}
	target := config.URL
	if target == "" {
		target = config.Webhook
	}
	if err := validateTarget(target); err != nil {
		return err
	}
	body := replace(config.Body, message)
	if dingTalk {
		encoded, _ := json.Marshal(map[string]any{"msgtype": "text", "text": map[string]string{"content": replace("📄 内容：{{sms-body}}\n📞 来自：{{sms-from}}\n⏰ 时间：{{sms-time}}", message)}})
		body = string(encoded)
	}
	method := strings.ToUpper(config.Method)
	if method == "" {
		method = http.MethodPost
	}
	if method != http.MethodPost && method != http.MethodPut {
		return errors.New("forwarding method must be POST or PUT")
	}
	req, err := http.NewRequestWithContext(ctx, method, target, strings.NewReader(body))
	if err != nil {
		return err
	}
	for key, value := range config.Headers {
		if strings.EqualFold(key, "Host") {
			continue
		}
		req.Header.Set(key, value)
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("forwarding endpoint returned %d", resp.StatusCode)
	}
	return nil
}

func (m *Manager) blacklisted(message Message) bool {
	configText, _ := m.setting("sms_forward_blacklist")
	var config struct {
		Phone    string   `json:"phone"`
		Keywords string   `json:"keywords"`
		Phones   []string `json:"phones"`
		Words    []string `json:"words"`
	}
	_ = json.Unmarshal([]byte(configText), &config)
	config.Phones = append(config.Phones, splitList(config.Phone)...)
	config.Words = append(config.Words, splitList(config.Keywords)...)
	for _, phone := range config.Phones {
		if strings.TrimSpace(phone) == message.From {
			return true
		}
	}
	for _, word := range config.Words {
		if word != "" && strings.Contains(message.Body, word) {
			return true
		}
	}
	return false
}

func (m *Manager) setting(key string) (string, error) {
	value, err := m.store.Get(context.Background(), "kv", key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

func parseBool(value string) bool {
	var direct bool
	if json.Unmarshal([]byte(value), &direct) == nil {
		return direct
	}
	var object map[string]any
	if json.Unmarshal([]byte(value), &object) == nil {
		for _, key := range []string{"enabled", "enable", "value"} {
			if candidate, ok := object[key]; ok {
				text := strings.ToLower(strings.TrimSpace(toString(candidate)))
				return text == "1" || text == "true" || text == "on"
			}
		}
	}
	parsed, _ := strconv.ParseBool(strings.Trim(value, `" `))
	return parsed
}

func settingString(value string, keys ...string) string {
	var direct string
	if json.Unmarshal([]byte(value), &direct) == nil {
		return strings.TrimSpace(direct)
	}
	var object map[string]any
	if json.Unmarshal([]byte(value), &object) == nil {
		for _, key := range keys {
			if candidate, ok := object[key].(string); ok && strings.TrimSpace(candidate) != "" {
				return strings.TrimSpace(candidate)
			}
		}
	}
	return strings.Trim(value, `" `)
}

func replace(template string, message Message) string {
	if template == "" {
		encoded, _ := json.Marshal(map[string]string{"body": message.Body, "from": message.From, "time": message.Date})
		return string(encoded)
	}
	var parsed any
	if json.Unmarshal([]byte(template), &parsed) == nil {
		var walk func(any) any
		walk = func(v any) any {
			switch x := v.(type) {
			case string:
				return strings.NewReplacer("{{sms-body}}", message.Body, "{{sms-from}}", message.From, "{{sms-time}}", message.Date).Replace(x)
			case map[string]any:
				for k, item := range x {
					x[k] = walk(item)
				}
			case []any:
				for i, item := range x {
					x[i] = walk(item)
				}
			}
			return v
		}
		encoded, _ := json.Marshal(walk(parsed))
		return string(encoded)
	}
	replacer := strings.NewReplacer("{{sms-body}}", message.Body, "{{sms-from}}", message.From, "{{sms-time}}", message.Date)
	return replacer.Replace(template)
}

func validateTarget(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return errors.New("forwarding URL must use HTTP(S)")
	}
	if parsed.Scheme == "http" {
		host := parsed.Hostname()
		ip := net.ParseIP(host)
		if !strings.EqualFold(host, "localhost") && (ip == nil || !(ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast())) {
			return errors.New("plain HTTP forwarding requires a private address")
		}
	}
	return nil
}

func splitList(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ';' || r == '\n' })
}

func first(item map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := toString(item[key]); value != "" {
			return value
		}
	}
	return ""
}

func toString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	default:
		return fmt.Sprint(value)
	}
}
