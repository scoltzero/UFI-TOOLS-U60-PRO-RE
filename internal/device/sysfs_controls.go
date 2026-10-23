// SPDX-License-Identifier: AGPL-3.0-or-later
package device

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Local sysfs writes are limited to the requested feature's documented files.
// Telemetry.Root supplies an isolated filesystem in tests.
func (u *U60UBUS) sysPaths(pattern string) []string {
	if u.local == nil || u.metrics == nil {
		return nil
	}
	root := u.metrics.Root
	if root == "" {
		root = "/"
	}
	paths, _ := filepath.Glob(filepath.Join(root, pattern))
	return paths
}
func (u *U60UBUS) governorPaths() []string {
	paths := u.sysPaths("sys/devices/system/cpu/cpufreq/policy*/scaling_governor")
	if len(paths) == 0 {
		paths = u.sysPaths("sys/devices/system/cpu/cpu[0-9]*/cpufreq/scaling_governor")
	}
	return paths
}
func writeSysValues(values map[string]string) error {
	before := map[string]string{}
	for path := range values {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		before[path] = string(data)
	}
	changed := []string{}
	for path, value := range values {
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			failures := []error{err}
			for _, old := range changed {
				if err := os.WriteFile(old, []byte(before[old]), 0600); err != nil {
					failures = append(failures, fmt.Errorf("restore %s: %w", old, err))
				}
			}
			return errors.Join(failures...)
		}
		changed = append(changed, path)
	}
	return nil
}
func (u *U60UBUS) setPerformance(mode string) error {
	enabled, err := flagValue(mode)
	if err != nil {
		return err
	}
	paths := u.governorPaths()
	if len(paths) == 0 {
		return ErrUnsupported
	}
	values := map[string]string{}
	for _, path := range paths {
		data, err := os.ReadFile(filepath.Join(filepath.Dir(path), "scaling_available_governors"))
		if err != nil {
			return err
		}
		available := " " + strings.Join(strings.Fields(string(data)), " ") + " "
		choices := []string{"schedutil", "ondemand", "powersave"}
		if enabled == 1 {
			choices = []string{"performance"}
		}
		for _, choice := range choices {
			if strings.Contains(available, " "+choice+" ") {
				values[path] = choice
				break
			}
		}
		if values[path] == "" {
			return ErrUnsupported
		}
	}
	return writeSysValues(values)
}
func (u *U60UBUS) setLights(mode string) error {
	enabled, err := flagValue(mode)
	if err != nil {
		return err
	}
	paths := u.sysPaths("sys/class/leds/*/brightness")
	if len(paths) == 0 {
		return ErrUnsupported
	}
	values := map[string]string{}
	for _, path := range paths {
		value := "0"
		if enabled == 1 {
			data, err := os.ReadFile(filepath.Join(filepath.Dir(path), "max_brightness"))
			if err != nil {
				return err
			}
			value = strings.TrimSpace(string(data))
		}
		values[path] = value
	}
	return writeSysValues(values)
}
func (u *U60UBUS) systemSnapshot(keys []string) map[string]any {
	out := map[string]any{}
	for _, key := range keys {
		switch key {
		case "performance_mode":
			paths := u.governorPaths()
			if len(paths) == 0 {
				continue
			}
			mode := "1"
			valid := true
			for _, path := range paths {
				data, err := os.ReadFile(path)
				if err != nil {
					valid = false
					break
				}
				if strings.TrimSpace(string(data)) != "performance" {
					mode = "0"
				}
			}
			if valid {
				out[key] = mode
			}
		case "indicator_light_switch":
			paths := u.sysPaths("sys/class/leds/*/brightness")
			if len(paths) == 0 {
				continue
			}
			mode := "0"
			for _, path := range paths {
				data, err := os.ReadFile(path)
				if err == nil && strings.TrimSpace(string(data)) != "0" {
					mode = "1"
				}
			}
			out[key] = mode
		}
	}
	return out
}
