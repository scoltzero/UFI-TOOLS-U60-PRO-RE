package logfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBoundedRotationDisableAndClear(t *testing.T) {
	on := true
	f := &File{Path: filepath.Join(t.TempDir(), "candidate.log"), MaxBytes: 16, Enabled: func() bool { return on }}
	_, _ = f.Write([]byte("first-line\n"))
	_, _ = f.Write([]byte("second-line\n"))
	before, _ := f.Read()
	if !strings.Contains(string(before), "second") {
		t.Fatal(string(before))
	}
	if st, e := os.Stat(f.Path + ".1"); e != nil || st.Size() > 16 {
		t.Fatal("rotation not bounded")
	}
	on = false
	_, _ = f.Write([]byte("disabled"))
	after, _ := f.Read()
	if string(after) != string(before) {
		t.Fatal("disabled logs written")
	}
	if err := f.Clear(); err != nil {
		t.Fatal(err)
	}
	if data, err := f.Read(); err != nil || len(data) != 0 {
		t.Fatal("clear failed")
	}
}
