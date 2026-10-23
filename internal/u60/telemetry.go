// SPDX-License-Identifier: AGPL-3.0-or-later
package u60

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

// Telemetry reads the Linux system that runs the candidate. Root is injectable
// for filesystem fixtures; production uses /. Missing sensors remain nil.
type Telemetry struct {
	Root     string
	mu       sync.Mutex
	previous map[string]cpuTicks
}

type cpuTicks struct{ total, idle uint64 }

func (t *Telemetry) path(name string) string {
	root := t.Root
	if root == "" {
		root = "/"
	}
	return filepath.Join(root, name)
}

func (t *Telemetry) number(name string) (float64, bool) {
	b, err := os.ReadFile(t.path(name))
	if err != nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
	return v, err == nil && !math.IsNaN(v) && !math.IsInf(v, 0)
}

func (t *Telemetry) Read() map[string]any {
	t.mu.Lock()
	defer t.mu.Unlock()
	m := map[string]any{
		"power_connected": nil, "battery": nil, "battery_temperature": nil, "current_now": nil, "voltage_now": nil,
		"cpu_temp": nil, "cpu_usage": nil, "mem_usage": nil,
		"internal_available_storage": nil, "internal_used_storage": nil, "internal_total_storage": nil,
		"cpuFreqInfo": map[string]any{}, "cpuUsageInfo": map[string]any{},
		"cpu_temp_list": []any{}, "memInfo": map[string]any{},
	}
	t.readCPU(m)
	t.readMemory(m)
	t.readConnections(m)
	t.readSensors(m)
	for _, source := range []string{"usb", "ac", "usb-main"} {
		if v, ok := t.number("sys/class/power_supply/" + source + "/online"); ok {
			before, _ := m["power_connected"].(bool)
			m["power_connected"] = before || v == 1
		}
	}
	for _, dir := range []string{"data", "overlay", ""} {
		var st unix.Statfs_t
		if err := unix.Statfs(t.path(dir), &st); err == nil && st.Blocks > 0 {
			total := uint64(st.Blocks) * uint64(st.Bsize)
			available := uint64(st.Bavail) * uint64(st.Bsize)
			m["internal_total_storage"] = total
			m["internal_available_storage"] = available
			m["internal_used_storage"] = uint64(st.Blocks-st.Bfree) * uint64(st.Bsize)
			break
		}
	}
	return m
}

func (t *Telemetry) readCPU(m map[string]any) {
	b, err := os.ReadFile(t.path("proc/stat"))
	if err == nil {
		next := map[string]cpuTicks{}
		perCPU := map[string]any{}
		s := bufio.NewScanner(strings.NewReader(string(b)))
		for s.Scan() {
			f := strings.Fields(s.Text())
			if len(f) < 5 || !strings.HasPrefix(f[0], "cpu") {
				continue
			}
			var ticks cpuTicks
			valid := true
			// guest counters are already included in user/nice, not added twice.
			for i := 1; i < len(f) && i <= 8; i++ {
				v, e := strconv.ParseUint(f[i], 10, 64)
				if e != nil {
					valid = false
					break
				}
				ticks.total += v
				if i == 4 || i == 5 {
					ticks.idle += v
				}
			}
			if !valid {
				continue
			}
			next[f[0]] = ticks
			if old, ok := t.previous[f[0]]; ok && ticks.total > old.total && ticks.idle >= old.idle {
				delta, idle := ticks.total-old.total, ticks.idle-old.idle
				if idle > delta {
					continue
				}
				percent := float64(delta-idle) * 100 / float64(delta)
				if f[0] == "cpu" {
					m["cpu_usage"] = percent
				} else {
					perCPU[f[0]] = percent
				}
			}
		}
		t.previous = next
		m["cpuUsageInfo"] = perCPU
	}
	freq := map[string]any{}
	dirs, _ := filepath.Glob(t.path("sys/devices/system/cpu/cpu[0-9]*"))
	for _, dir := range dirs {
		base, _ := filepath.Rel(t.path(""), dir)
		entry := map[string]any{}
		for _, item := range [][2]string{{"cur", "scaling_cur_freq"}, {"max", "cpuinfo_max_freq"}} {
			if v, ok := t.number(filepath.Join(base, "cpufreq", item[1])); ok && v >= 0 {
				entry[item[0]] = v / 1000
			}
		}
		if len(entry) > 0 {
			freq[filepath.Base(dir)] = entry
		}
	}
	m["cpuFreqInfo"] = freq
}

func (t *Telemetry) readMemory(m map[string]any) {
	b, err := os.ReadFile(t.path("proc/meminfo"))
	if err != nil {
		return
	}
	v := map[string]uint64{}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 {
			n, e := strconv.ParseUint(f[1], 10, 64)
			if e == nil {
				v[strings.TrimSuffix(f[0], ":")] = n
			}
		}
	}
	total, haveTotal := v["MemTotal"]
	available, haveAvailable := v["MemAvailable"]
	if !haveTotal || !haveAvailable || total == 0 || available > total {
		return
	}
	used := total - available
	percent := float64(used) * 100 / float64(total)
	m["mem_usage"] = percent
	m["memInfo"] = map[string]any{"mem_total_kb": total, "mem_available_kb": available, "mem_used_kb": used, "mem_usage_percent": percent}
}

func (t *Telemetry) readSensors(m map[string]any) {
	for _, dir := range []string{"sys/class/power_supply/battery", "sys/class/power_supply/bms"} {
		if _, err := os.Stat(t.path(dir)); err != nil {
			continue
		}
		for _, item := range [][2]string{{"battery", "capacity"}, {"battery_temperature", "temp"}, {"current_now", "current_now"}, {"voltage_now", "voltage_now"}} {
			if v, ok := t.number(filepath.Join(dir, item[1])); ok {
				if item[0] == "battery_temperature" {
					v *= 100
				} // tenths Celsius -> milli Celsius
				m[item[0]] = v
			}
		}
		break
	}
	files, _ := filepath.Glob(t.path("sys/class/thermal/thermal_zone*/temp"))
	list := []any{}
	var maximum float64
	foundCPU := false
	for _, file := range files {
		name, _ := filepath.Rel(t.path(""), file)
		if v, ok := t.number(name); ok && v > 0 && v < 200000 {
			list = append(list, v)
			kind, _ := os.ReadFile(filepath.Join(filepath.Dir(file), "type"))
			label := strings.ToLower(strings.TrimSpace(string(kind)))
			if strings.Contains(label, "cpu") || strings.Contains(label, "soc") || strings.Contains(label, "tsens") {
				if !foundCPU || v > maximum {
					maximum = v
				}
				foundCPU = true
			}
		}
	}
	if foundCPU {
		m["cpu_temp"] = fmt.Sprint(maximum)
	}
	m["cpu_temp_list"] = list
}

func (t *Telemetry) readConnections(m map[string]any) {
	result := map[string]any{}
	for _, kind := range []string{"tcp", "tcp6", "udp", "udp6", "unix"} {
		data, err := os.ReadFile(t.path("proc/net/" + kind))
		if err != nil {
			continue
		}
		total, active := 0, 0
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n")[1:] {
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			total++
			if kind == "tcp" && len(fields) > 3 && fields[3] == "01" {
				active++
			}
		}
		result[kind] = total
		if kind == "tcp" {
			result["tcp_active"] = active
			result["tcp_other"] = total - active
		}
	}
	if len(result) > 0 {
		m["connections"] = result
	}
}
