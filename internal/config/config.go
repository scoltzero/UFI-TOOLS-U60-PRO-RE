// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultDataDir  = "/data/ufi-tools"
	defaultHTTPAddr = ":2333"
)

type Config struct {
	DataDir            string
	HTTPAddr           string
	LogLevelName       string
	Nickname           string
	DeviceToken        string
	LegacyAuthEnabled  bool
	LegacyShellEnabled bool
	PluginServerURL    string
	ResourceServerURL  string
	SignClockSkew      time.Duration
}

func Load() (Config, error) {
	cfg := Defaults()

	flags := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	flags.StringVar(&cfg.DataDir, "data-dir", envString("UFI_DATA_DIR", cfg.DataDir), "persistent data directory")
	flags.StringVar(&cfg.HTTPAddr, "http-addr", envString("UFI_HTTP_ADDR", cfg.HTTPAddr), "HTTP listen address")
	flags.StringVar(&cfg.LogLevelName, "log-level", envString("UFI_LOG_LEVEL", cfg.LogLevelName), "debug, info, warn, or error")
	flags.StringVar(&cfg.Nickname, "nickname", envString("UFI_NICKNAME", cfg.Nickname), "device display name")
	flags.StringVar(&cfg.DeviceToken, "device-token", envString("UFI_DEVICE_TOKEN", cfg.DeviceToken), "API bearer token")
	flags.BoolVar(&cfg.LegacyAuthEnabled, "legacy-auth", envBool("UFI_LEGACY_AUTH", cfg.LegacyAuthEnabled), "enable legacy authentication compatibility")
	flags.BoolVar(&cfg.LegacyShellEnabled, "legacy-shell", envBool("UFI_LEGACY_SHELL", cfg.LegacyShellEnabled), "enable legacy shell endpoint")
	flags.StringVar(&cfg.PluginServerURL, "plugin-server-url", envString("UFI_PLUGIN_SERVER_URL", cfg.PluginServerURL), "optional plugin catalog base URL")
	flags.StringVar(&cfg.ResourceServerURL, "resource-server-url", envString("UFI_RESOURCE_SERVER_URL", cfg.ResourceServerURL), "optional resource base URL")
	skew, err := time.ParseDuration(envString("UFI_SIGN_CLOCK_SKEW", cfg.SignClockSkew.String()))
	if err != nil {
		return Config{}, fmt.Errorf("UFI_SIGN_CLOCK_SKEW: %w", err)
	}
	flags.DurationVar(&cfg.SignClockSkew, "sign-clock-skew", skew, "accepted legacy signature clock skew")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Defaults() Config {
	return Config{
		DataDir:           defaultDataDir,
		HTTPAddr:          defaultHTTPAddr,
		LogLevelName:      "info",
		Nickname:          "U60 Pro",
		SignClockSkew:     5 * time.Minute,
		PluginServerURL:   "",
		ResourceServerURL: "",
	}
}

func (c Config) Validate() error {
	if c.DataDir == "" || !filepath.IsAbs(c.DataDir) {
		return errors.New("data directory must be an absolute path")
	}
	if _, _, err := net.SplitHostPort(c.HTTPAddr); err != nil {
		return fmt.Errorf("invalid HTTP address %q: %w", c.HTTPAddr, err)
	}
	switch strings.ToLower(c.LogLevelName) {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("invalid log level %q", c.LogLevelName)
	}
	if c.SignClockSkew < 0 || c.SignClockSkew > time.Hour {
		return errors.New("signature clock skew must be between zero and one hour")
	}
	if c.LegacyShellEnabled && !c.LegacyAuthEnabled {
		return errors.New("legacy shell requires legacy authentication")
	}
	return nil
}

func (c Config) LogLevel() slog.Level {
	switch strings.ToLower(c.LogLevelName) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func envString(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok {
		return value
	}
	return fallback
}

func envBool(name string, fallback bool) bool {
	value, ok := os.LookupEnv(name)
	if !ok {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
