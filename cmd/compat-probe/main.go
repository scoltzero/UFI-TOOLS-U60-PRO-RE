// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/probe"
)

func main() {
	base := flag.String("base", "http://127.0.0.1:22333", "private/loopback service origin")
	tokenFile := flag.String("token-file", "", "JSON configuration containing device_token, or a plain token file")
	output := flag.String("output", "", "write the sanitized JSON fixture to this path (stdout when empty)")
	timeout := flag.Duration("timeout", 2*time.Minute, "overall capture timeout")
	flag.Parse()

	token, err := readToken(*tokenFile)
	fatal(err)
	client, err := probe.New(*base, token, nil)
	fatal(err)
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	result := client.Capture(ctx, flag.Args())

	w := os.Stdout
	if *output != "" {
		w, err = os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
		fatal(err)
		defer w.Close()
	}
	fatal(probe.Encode(w, result))
}

func readToken(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var config struct {
		DeviceToken string `json:"device_token"`
	}
	if json.Unmarshal(data, &config) == nil && config.DeviceToken != "" {
		return config.DeviceToken, nil
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("token file is empty")
	}
	return token, nil
}

func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
