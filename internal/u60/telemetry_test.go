package u60

import (
	"os"
	"path/filepath"
	"testing"
)

func fixtureFile(t *testing.T, root, name, value string) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestTelemetryUnitsAndCPUIntervals(t *testing.T) {
	root := t.TempDir()
	fixtureFile(t, root, "proc/stat", "cpu 100 0 50 850 0 0 0 0\ncpu0 100 0 50 850 0 0 0 0\n")
	fixtureFile(t, root, "proc/meminfo", "MemTotal: 1000 kB\nMemAvailable: 400 kB\n")
	fixtureFile(t, root, "sys/class/power_supply/battery/capacity", "90")
	fixtureFile(t, root, "sys/class/power_supply/battery/temp", "365")
	fixtureFile(t, root, "sys/class/power_supply/battery/current_now", "-125000")
	fixtureFile(t, root, "sys/class/power_supply/battery/voltage_now", "4200000")
	fixtureFile(t, root, "sys/class/thermal/thermal_zone0/temp", "-273000")
	fixtureFile(t, root, "sys/class/thermal/thermal_zone1/temp", "42000")
	fixtureFile(t, root, "sys/class/thermal/thermal_zone1/type", "cpu-thermal")
	fixtureFile(t, root, "sys/class/thermal/thermal_zone2/temp", "65000")
	fixtureFile(t, root, "sys/class/thermal/thermal_zone2/type", "battery")
	fixtureFile(t, root, "sys/devices/system/cpu/cpu0/cpufreq/scaling_cur_freq", "1200000")
	fixtureFile(t, root, "sys/devices/system/cpu/cpu0/cpufreq/cpuinfo_max_freq", "2208000")
	c := &Telemetry{Root: root}
	first := c.Read()
	if first["cpu_usage"] != nil {
		t.Fatal("first CPU interval must be unavailable")
	}
	if first["battery_temperature"] != float64(36500) || first["current_now"] != float64(-125000) || first["voltage_now"] != float64(4200000) {
		t.Fatalf("incorrect battery units: %#v", first)
	}
	if first["cpu_temp"] != "42000" || first["mem_usage"] != float64(60) {
		t.Fatalf("incorrect sensors: %#v", first)
	}
	freq := first["cpuFreqInfo"].(map[string]any)["cpu0"].(map[string]any)
	if freq["cur"] != float64(1200) || freq["max"] != float64(2208) {
		t.Fatalf("frequency %#v", freq)
	}
	fixtureFile(t, root, "proc/stat", "cpu 150 0 50 900 0 0 0 0\ncpu0 150 0 50 900 0 0 0 0\n")
	if next := c.Read(); next["cpu_usage"] != float64(50) {
		t.Fatalf("CPU interval=%v", next["cpu_usage"])
	}
	fixtureFile(t, root, "proc/stat", "cpu 1 0 1 8 0 0 0 0\n")
	if reset := c.Read(); reset["cpu_usage"] != nil {
		t.Fatal("counter reset must not report a bogus percent")
	}
}

func TestMissingSensorsRemainUnavailable(t *testing.T) {
	m := (&Telemetry{Root: t.TempDir()}).Read()
	for _, key := range []string{"battery", "battery_temperature", "cpu_usage", "cpu_temp", "mem_usage", "current_now", "voltage_now"} {
		if m[key] != nil {
			t.Fatalf("missing %s=%v", key, m[key])
		}
	}
}
