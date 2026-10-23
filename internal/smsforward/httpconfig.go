// SPDX-License-Identifier: AGPL-3.0-or-later
package smsforward

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// ParseHTTPConfig accepts the existing curl input without executing a shell.
// Only a URL, method, headers and literal request body are supported.
func ParseHTTPConfig(text string) (map[string]any, error) {
	var object map[string]any
	if json.Unmarshal([]byte(text), &object) == nil {
		target, _ := object["url"].(string)
		if err := validateTarget(target); err != nil {
			return nil, err
		}
		return object, nil
	}
	fields := []string{}
	var word strings.Builder
	var quote rune
	escaped := false
	started := false
	for _, ch := range strings.TrimSpace(text) {
		if escaped {
			word.WriteRune(ch)
			escaped = false
			started = true
			continue
		}
		if ch == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if ch == quote {
				quote = 0
			} else {
				word.WriteRune(ch)
			}
			started = true
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			started = true
			continue
		}
		if ch == ' ' || ch == '\n' || ch == '\t' || ch == '\r' {
			if started {
				fields = append(fields, word.String())
				word.Reset()
				started = false
			}
			continue
		}
		word.WriteRune(ch)
		started = true
	}
	if quote != 0 || escaped {
		return nil, errors.New("unterminated curl argument")
	}
	if started {
		fields = append(fields, word.String())
	}
	if len(fields) > 0 && fields[0] == "curl" {
		fields = fields[1:]
	}
	out := map[string]any{}
	headers := map[string]string{}
	for i := 0; i < len(fields); i++ {
		arg := fields[i]
		switch arg {
		case "-s", "--silent", "-S", "--show-error":
			continue
		case "-X", "--request", "-H", "--header", "-d", "--data", "--data-raw", "--data-binary", "--url":
			i++
			if i >= len(fields) {
				return nil, errors.New("curl option requires a value")
			}
			value := fields[i]
			switch arg {
			case "-X", "--request":
				out["method"] = strings.ToUpper(value)
			case "-H", "--header":
				key, val, ok := strings.Cut(value, ":")
				if !ok {
					return nil, errors.New("invalid forwarding header")
				}
				headers[strings.TrimSpace(key)] = strings.TrimSpace(val)
			case "--url":
				out["url"] = value
			default:
				if strings.HasPrefix(value, "@") {
					return nil, errors.New("file-based curl bodies are unsupported")
				}
				out["body"] = value
			}
		default:
			if strings.HasPrefix(arg, "-") || out["url"] != nil {
				return nil, errors.New("unsupported curl option or multiple URLs")
			}
			out["url"] = arg
		}
	}
	target, _ := out["url"].(string)
	if err := validateTarget(target); err != nil {
		return nil, err
	}
	if method, ok := out["method"].(string); ok && method != http.MethodPost && method != http.MethodPut {
		return nil, errors.New("forwarding method must be POST or PUT")
	}
	out["headers"] = headers
	return out, nil
}
