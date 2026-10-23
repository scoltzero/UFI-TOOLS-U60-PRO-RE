package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestUsageBaselineResetUTCAndRetention(t *testing.T) {
	db, err := Open(t.Context(), filepath.Join(t.TempDir(), "usage.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	start := time.Date(2026, 10, 21, 23, 59, 0, 0, time.UTC).Unix()
	for _, sample := range [][2]int64{{start, 5000}, {start + 30, 5100}, {start + 90, 5250}, {start + 120, 25}, {start + 150, 30}} {
		if err = db.RecordUsage(t.Context(), sample[0], sample[1]); err != nil {
			t.Fatal(err)
		}
	}
	days, err := db.DailyUsage(t.Context(), "2026-10-21", "2026-10-22")
	if err != nil || len(days) != 2 || days[0].Usage != 100 || days[1].Usage != 180 {
		t.Fatalf("days=%v err=%v", days, err)
	}
	if err = db.RecordUsage(t.Context(), start+150, 30); err == nil {
		t.Fatal("duplicate sample counted")
	}
	if err = db.RecordUsage(t.Context(), start+8*86400, 40); err != nil {
		t.Fatal(err)
	}
	points, err := db.RecentCellular(t.Context(), 1000)
	if err != nil || len(points) != 1 {
		t.Fatalf("unbounded raw points: %v %v", points, err)
	}
}
