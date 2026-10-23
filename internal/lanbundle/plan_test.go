package lanbundle

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/ztebackup"
)

func fixture(t *testing.T) (string, []byte, []byte, []byte) {
	t.Helper()
	dir := t.TempDir()
	key := []byte("fixture-backup-key")
	rc := []byte("#!/bin/sh\n# stock setup\nsh /data/ufi-tools/service.sh start\nexit 0")
	backup, err := (ztebackup.Builder{Password: key}).Build([]ztebackup.File{{Path: RCPath, Mode: 0o750, Data: rc}})
	if err != nil {
		t.Fatal(err)
	}
	bin := make([]byte, 24)
	copy(bin, []byte{0x7f, 'E', 'L', 'F', 2, 1})
	bin[18] = 183
	files := map[string][]byte{"ufi-tools-u60pro": bin, "service.sh": []byte("#!/bin/sh\n# lan-bootstrap fixture\n")}
	var manifest strings.Builder
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
		manifest.WriteString(Digest(data) + "  " + name + "\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "SHA256SUMS"), []byte(manifest.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, key, backup, rc
}

func TestPrepareWritesOnlyStageAndBootBaseline(t *testing.T) {
	dir, key, baseline, rc := fixture(t)
	p, candidate, rollback, err := Prepare(dir, baseline, key, "install", "")
	if err != nil {
		t.Fatal(err)
	}
	if p.DeploymentID != p.ID || p.BaselineSHA256 != Digest(rc) {
		t.Fatal("bad identity")
	}
	if err := ValidateArtifacts(p, candidate, rollback, key); err != nil {
		t.Fatal(err)
	}
	modified := p
	modified.Paths = []string{RCPath}
	if err := ValidateArtifacts(modified, candidate, rollback, key); err == nil {
		t.Fatal("accepted misleading path preview")
	}
	for _, artifact := range [][]byte{candidate, rollback} {
		report, err := ztebackup.Inspect(artifact, key)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range report.Paths {
			if name != RCPath && !strings.HasPrefix(name, "data"+Root+".lan-stage-"+p.ID) {
				t.Fatalf("unexpected modified path: %s", name)
			}
		}
		boot, err := ztebackup.ReadFile(artifact, key, RCPath)
		if err != nil {
			t.Fatal(err)
		}
		if boot.Mode != 0o750 || !bytes.Contains(boot.Data, []byte("sh /data/ufi-tools/service.sh start")) {
			t.Fatal("original startup/mode lost")
		}
	}
	before, err := ztebackup.ReadFile(candidate, key, "data"+Root+".lan-stage-"+p.ID+"/lan-rc-before")
	if err != nil || !bytes.Equal(before.Data, rc) {
		t.Fatal("original bytes not preserved")
	}
	output := filepath.Join(t.TempDir(), "plan")
	if err = Save(output, p, candidate, rollback); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = Load(output); err != nil {
		t.Fatal(err)
	}
	if err = Save(output, p, candidate, rollback); err == nil {
		t.Fatal("overwrote existing plan")
	}
	os.WriteFile(filepath.Join(output, "candidate.bin"), []byte("tampered"), 0o600)
	if _, _, _, err = Load(output); err == nil {
		t.Fatal("accepted modified artifact")
	}
}

func TestUninstallHookReplacesCandidateLineBeforeSetECanAbort(t *testing.T) {
	before := []byte("#!/bin/sh\nset -e\nsh /data/ufi-tools/service.sh start\nsh " + Root + "/service.sh start\necho other\nexit 0\n")
	after, err := PatchBoot(before, "sh staged-controller")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(after, []byte("sh "+Root+"/service.sh start")) || !bytes.Contains(after, []byte("sh staged-controller\necho other")) {
		t.Fatal("dangling candidate start after uninstall")
	}
	for _, bad := range []string{"exit 0\necho unreachable\n", "exit 1\n", Root + ".lan-stage-old\n"} {
		if _, err := PatchBoot([]byte(bad), "hook"); err == nil {
			t.Fatalf("accepted ambiguous boot: %s", bad)
		}
	}
}

func TestPrepareRejectsTamperedBundleAndWrongKey(t *testing.T) {
	dir, key, baseline, _ := fixture(t)
	if _, _, _, err := Prepare(dir, baseline, []byte("wrong"), "install", ""); err == nil {
		t.Fatal("accepted wrong backup key")
	}
	os.WriteFile(filepath.Join(dir, "service.sh"), []byte("changed"), 0o600)
	if _, _, _, err := Prepare(dir, baseline, key, "install", ""); err == nil {
		t.Fatal("accepted tampered script")
	}
}

func TestPrepareUninstallRequiresReceipt(t *testing.T) {
	dir, key, baseline, _ := fixture(t)
	if _, _, _, err := Prepare(dir, baseline, key, "uninstall", ""); err == nil {
		t.Fatal("accepted unidentified uninstall")
	}
	p, _, rollback, err := Prepare(dir, baseline, key, "uninstall", strings.Repeat("a", 32))
	if err != nil {
		t.Fatal(err)
	}
	action, err := ztebackup.ReadFile(rollback, key, "data"+Root+".lan-stage-"+p.ID+"-recovery/lan-action")
	if err != nil || string(action.Data) != "restore\n" {
		t.Fatal("uninstall has no restore inverse")
	}
}
