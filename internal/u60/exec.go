// SPDX-License-Identifier: AGPL-3.0-or-later

// Package u60 talks to the U60 Pro system services the way the reference
// service does: through the ubus and uci command-line tools.
package u60

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Runner executes one command and returns its combined stdout and stderr.
type Runner interface {
	Run(ctx context.Context, timeout time.Duration, name string, args ...string) ([]byte, error)
}

// LocalRunner runs commands on the host the service runs on.
type LocalRunner struct{}

func (LocalRunner) Run(ctx context.Context, timeout time.Duration, name string, args ...string) ([]byte, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return out.Bytes(), fmt.Errorf("%s timeout after %s", name, timeout)
	}
	if err != nil {
		return out.Bytes(), fmt.Errorf("%s: %w (output=%q)", name, err, strings.TrimSpace(out.String()))
	}
	return out.Bytes(), nil
}

// RemoteRunner runs commands through a prefix such as `adb -s SERIAL shell`.
// It exists for development against a real device from another host; every
// argument is single-quoted for the remote shell.
type RemoteRunner struct {
	Prefix []string
}

func (r RemoteRunner) Run(ctx context.Context, timeout time.Duration, name string, args ...string) ([]byte, error) {
	if len(r.Prefix) == 0 {
		return nil, fmt.Errorf("remote runner has no prefix")
	}
	quoted := make([]string, 0, len(args)+1)
	for _, arg := range append([]string{name}, args...) {
		quoted = append(quoted, "'"+strings.ReplaceAll(arg, "'", `'\''`)+"'")
	}
	remote := strings.Join(quoted, " ") + "; echo \"__rc=$?\""
	out, err := LocalRunner{}.Run(ctx, timeout, r.Prefix[0], append(r.Prefix[1:], remote)...)
	if err != nil {
		return out, err
	}
	text := strings.ReplaceAll(string(out), "\r\n", "\n")
	i := strings.LastIndex(text, "__rc=")
	if i < 0 {
		return []byte(text), fmt.Errorf("%s: missing exit status", name)
	}
	body, rc := text[:i], strings.TrimSpace(text[i+len("__rc="):])
	if rc != "0" {
		return []byte(body), fmt.Errorf("%s: exit status %s (output=%q)", name, rc, strings.TrimSpace(body))
	}
	return []byte(body), nil
}
