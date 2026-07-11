// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/url"
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
	PluginServerToken  string
	ResourceServerURL  string
	SignClockSkew      time.Duration
	DeviceBackend      string
	DeviceBaseURL      string
	DeviceHTTPTimeout  time.Duration
	WorkersEnabled     bool
	SchedulerInterval  time.Duration
	CellularInterval   time.Duration
	ShellTimeout       time.Duration
	ShellMaxOutput     int
	TTYDBinary         string
	TTYDPort           int
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
	flags.StringVar(&cfg.PluginServerToken, "plugin-server-token", envString("UFI_PLUGIN_SERVER_TOKEN", cfg.PluginServerToken), "optional plugin catalog token")
	flags.StringVar(&cfg.ResourceServerURL, "resource-server-url", envString("UFI_RESOURCE_SERVER_URL", cfg.ResourceServerURL), "optional resource base URL")
	flags.StringVar(&cfg.DeviceBackend, "device-backend", envString("UFI_DEVICE_BACKEND", cfg.DeviceBackend), "simulator or zte-http")
	flags.StringVar(&cfg.DeviceBaseURL, "device-base-url", envString("UFI_DEVICE_BASE_URL", cfg.DeviceBaseURL), "ZTE device HTTP origin")
	deviceTimeout, err := time.ParseDuration(envString("UFI_DEVICE_HTTP_TIMEOUT", cfg.DeviceHTTPTimeout.String()))
	if err != nil {
		return Config{}, fmt.Errorf("UFI_DEVICE_HTTP_TIMEOUT: %w", err)
	}
	flags.DurationVar(&cfg.DeviceHTTPTimeout, "device-http-timeout", deviceTimeout, "ZTE HTTP request timeout")
	flags.BoolVar(&cfg.WorkersEnabled, "workers", envBool("UFI_WORKERS_ENABLED", cfg.WorkersEnabled), "enable scheduler and cellular workers")
	schedulerInterval, err := time.ParseDuration(envString("UFI_SCHEDULER_INTERVAL", cfg.SchedulerInterval.String()))
	if err != nil {
		return Config{}, fmt.Errorf("UFI_SCHEDULER_INTERVAL: %w", err)
	}
	flags.DurationVar(&cfg.SchedulerInterval, "scheduler-interval", schedulerInterval, "scheduler polling interval")
	cellularInterval, err := time.ParseDuration(envString("UFI_CELLULAR_INTERVAL", cfg.CellularInterval.String()))
	if err != nil {
		return Config{}, fmt.Errorf("UFI_CELLULAR_INTERVAL: %w", err)
	}
	flags.DurationVar(&cfg.CellularInterval, "cellular-interval", cellularInterval, "cellular sample interval")
	shellTimeout, err := time.ParseDuration(envString("UFI_SHELL_TIMEOUT", cfg.ShellTimeout.String()))
	if err != nil {
		return Config{}, fmt.Errorf("UFI_SHELL_TIMEOUT: %w", err)
	}
	flags.DurationVar(&cfg.ShellTimeout, "shell-timeout", shellTimeout, "legacy shell command timeout")
	flags.IntVar(&cfg.ShellMaxOutput, "shell-max-output", envInt("UFI_SHELL_MAX_OUTPUT", cfg.ShellMaxOutput), "legacy shell maximum combined output bytes")
	flags.StringVar(&cfg.TTYDBinary, "ttyd-binary", envString("UFI_TTYD_BINARY", cfg.TTYDBinary), "ttyd binary path")
	flags.IntVar(&cfg.TTYDPort, "ttyd-port", envInt("UFI_TTYD_PORT", cfg.TTYDPort), "ttyd listen port")
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
		DeviceBackend:     "simulator",
		DeviceHTTPTimeout: 5 * time.Second,
		SchedulerInterval: 5 * time.Second,
		CellularInterval:  5 * time.Minute,
		ShellTimeout:      30 * time.Second,
		ShellMaxOutput:    1 << 20,
		TTYDBinary:        "/data/ufi-tools/ttyd",
		TTYDPort:          1146,
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
	if c.PluginServerURL != "" {
		pluginURL, err := url.Parse(c.PluginServerURL)
		if err != nil || pluginURL.Host == "" || (pluginURL.Scheme != "http" && pluginURL.Scheme != "https") {
			return errors.New("plugin server URL must be an HTTP(S) origin")
		}
		if pluginURL.Scheme == "http" {
			host := pluginURL.Hostname()
			ip := net.ParseIP(host)
			if !strings.EqualFold(host, "localhost") && (ip == nil || !(ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast())) {
				return errors.New("plain HTTP plugin server must use a private, loopback, or link-local address")
			}
		}
	}
	if c.DeviceHTTPTimeout <= 0 || c.DeviceHTTPTimeout > time.Minute {
		return errors.New("device HTTP timeout must be between zero and one minute")
	}
	if c.SchedulerInterval < time.Second || c.SchedulerInterval > time.Hour {
		return errors.New("scheduler interval must be between one second and one hour")
	}
	if c.CellularInterval < time.Second || c.CellularInterval > 24*time.Hour {
		return errors.New("cellular interval must be between one second and 24 hours")
	}
	if c.ShellTimeout < time.Second || c.ShellTimeout > 10*time.Minute {
		return errors.New("shell timeout must be between one second and ten minutes")
	}
	if c.ShellMaxOutput < 1024 || c.ShellMaxOutput > 16<<20 {
		return errors.New("shell maximum output must be between 1 KiB and 16 MiB")
	}
	if !filepath.IsAbs(c.TTYDBinary) || c.TTYDPort < 1 || c.TTYDPort > 65535 {
		return errors.New("ttyd requires an absolute binary path and valid port")
	}
	switch c.DeviceBackend {
	case "simulator":
		if c.DeviceBaseURL != "" {
			return errors.New("device base URL requires the zte-http backend")
		}
	case "zte-http":
		if c.DeviceBaseURL == "" {
			return errors.New("zte-http backend requires a device base URL")
		}
		deviceURL, err := url.Parse(c.DeviceBaseURL)
		if err != nil || (deviceURL.Scheme != "http" && deviceURL.Scheme != "https") || deviceURL.Hostname() == "" {
			return errors.New("device base URL must be an HTTP(S) origin")
		}
		host := deviceURL.Hostname()
		ip := net.ParseIP(host)
		if !strings.EqualFold(host, "localhost") && (ip == nil || !(ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast())) {
			return errors.New("device base URL must use a literal private, loopback, or link-local address")
		}
	default:
		return fmt.Errorf("unsupported device backend %q", c.DeviceBackend)
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

func envInt(name string, fallback int) int {
	value, ok := os.LookupEnv(name)
	if !ok {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
