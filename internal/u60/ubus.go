// SPDX-License-Identifier: AGPL-3.0-or-later

package u60

import (
	"bufio"
	"context"
	"encoding/json"
	"strings"
	"time"
)

// UBUS calls `ubus call SERVICE METHOD JSON`. Like the reference it never
// returns a Go error: failures come back as a map holding an "error" string,
// and successful replies are decoded into map[string]any (numbers float64).
type UBUS struct {
	Runner  Runner
	Timeout time.Duration
}

func (u UBUS) Call(ctx context.Context, service, method string, params map[string]any) map[string]any {
	args := []string{"call", service, method}
	if len(params) != 0 {
		encoded, err := json.Marshal(params)
		if err != nil {
			return map[string]any{"error": "marshal params: " + err.Error()}
		}
		args = append(args, string(encoded))
	}
	out, err := u.Runner.Run(ctx, u.timeout(), "ubus", args...)
	if err != nil {
		message := strings.TrimSpace(string(out))
		if message == "" {
			message = err.Error()
		}
		return map[string]any{"error": message}
	}
	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		return map[string]any{"error": strings.TrimSpace(string(out))}
	}
	if result == nil {
		result = map[string]any{}
	}
	return result
}

func (u UBUS) timeout() time.Duration {
	if u.Timeout > 0 {
		return u.Timeout
	}
	return 5 * time.Second
}

// HasError reports whether a Call result is a failure.
func HasError(result map[string]any) bool {
	if result == nil {
		return true
	}
	if ok, present := result["success"].(bool); present && !ok {
		return true
	}
	value, present := result["error"]
	if !present || value == nil {
		return false
	}
	switch v := value.(type) {
	case string:
		return v != "" && v != "0" && v != "false"
	case float64:
		return v != 0
	case int:
		return v != 0
	case bool:
		return v
	default:
		return true
	}
}

// UCI wraps the uci command-line tool.
type UCI struct {
	Runner  Runner
	Timeout time.Duration
}

// Get returns the trimmed value of a uci path, or "" when it does not exist.
func (u UCI) Get(ctx context.Context, path string) (string, error) {
	out, err := u.Runner.Run(ctx, u.timeout(), "uci", "get", path)
	if err != nil {
		text := strings.TrimSpace(string(out))
		if strings.Contains(text, "Entry not found") || strings.Contains(err.Error(), "exit status") {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Show returns `uci show CONFIG` as path -> value with one level of single
// quotes removed.
func (u UCI) Show(ctx context.Context, config string) (map[string]string, error) {
	out, err := u.Runner.Run(ctx, u.timeout(), "uci", "show", config)
	if err != nil {
		return nil, err
	}
	return ParseUCIShow(string(out)), nil
}

func ParseUCIShow(text string) map[string]string {
	values := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		i := strings.IndexByte(line, '=')
		if i < 1 {
			continue
		}
		key, value := strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+1:])
		if len(value) > 1 && value[0] == '\'' && value[len(value)-1] == '\'' {
			value = value[1 : len(value)-1]
		}
		values[key] = value
	}
	return values
}

func (u UCI) timeout() time.Duration {
	if u.Timeout > 0 {
		return u.Timeout
	}
	return 5 * time.Second
}
