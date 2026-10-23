// SPDX-License-Identifier: AGPL-3.0-or-later
package lanbundle

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/ztebackup"
)

const Root = "/data/ufi-tools-re"
const RCPath = "data/etc/rc.local"

var hexID = regexp.MustCompile(`^[0-9a-f]{32}$`)
var safeFile = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

type Plan struct {
	LoginToken      string   `json:"-"`
	Schema          int      `json:"schema"`
	ID              string   `json:"id"`
	Action          string   `json:"action"`
	DeploymentID    string   `json:"deployment_id"`
	BaselineSHA256  string   `json:"baseline_rc_sha256"`
	BaselineMode    int64    `json:"baseline_rc_mode"`
	CandidateSHA256 string   `json:"candidate_sha256"`
	RollbackSHA256  string   `json:"rollback_sha256"`
	BinarySHA256    string   `json:"binary_sha256"`
	Paths           []string `json:"candidate_paths"`
}

func Digest(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }

func (p Plan) Validate() error {
	if p.Schema != 1 || !hexID.MatchString(p.ID) || !hexID.MatchString(p.DeploymentID) || (p.Action != "install" && p.Action != "uninstall") {
		return errors.New("invalid LAN plan identity or action")
	}
	if p.BaselineMode < 0 || p.BaselineMode > 0o7777 {
		return errors.New("invalid baseline mode")
	}
	for _, hash := range []string{p.BaselineSHA256, p.CandidateSHA256, p.RollbackSHA256, p.BinarySHA256} {
		if len(hash) != 64 || strings.Trim(hash, "0123456789abcdef") != "" {
			return errors.New("invalid plan digest")
		}
	}
	return nil
}

// Prepare creates a fresh-install or receipt-bound uninstall pair. The vendor
// archive contains only a unique staging tree and the exact patched rc.local.
func Prepare(bundle string, baseline, password []byte, action, deployment string) (Plan, []byte, []byte, error) {
	var p Plan
	if action != "install" && action != "uninstall" {
		return p, nil, nil, errors.New("action must be install or uninstall")
	}
	seed := make([]byte, 16)
	if _, err := rand.Read(seed); err != nil {
		return p, nil, nil, err
	}
	id := hex.EncodeToString(seed)
	if action == "install" {
		deployment = id
	} else if !hexID.MatchString(deployment) {
		return p, nil, nil, errors.New("uninstall requires the saved installation deployment ID")
	}
	rc, err := ztebackup.ReadFile(baseline, password, RCPath)
	if err != nil {
		return p, nil, nil, err
	}
	if len(rc.Data) > 128<<10 {
		return p, nil, nil, errors.New("rc.local is too large")
	}
	if action == "install" && bytes.Contains(rc.Data, []byte(Root+"/service.sh")) {
		return p, nil, nil, errors.New("candidate startup already exists; LAN first install requires a clean candidate baseline")
	}
	files, err := readBundle(bundle)
	if err != nil {
		return p, nil, nil, err
	}
	p = Plan{Schema: 1, ID: id, Action: action, DeploymentID: deployment, BaselineSHA256: Digest(rc.Data), BaselineMode: rc.Mode, BinarySHA256: Digest(files["ufi-tools-u60pro"])}
	if action == "install" {
		token := make([]byte, 32)
		if _, err := rand.Read(token); err != nil {
			return p, nil, nil, err
		}
		p.LoginToken = hex.EncodeToString(token)
		files["lan-login-token"] = []byte(p.LoginToken + "\n")
	}
	candidateAction, rollbackAction := "install", "remove"
	if action == "uninstall" {
		candidateAction, rollbackAction = "remove", "restore"
	}
	candidate, paths, err := container(files, rc, password, id, deployment, candidateAction)
	if err != nil {
		return p, nil, nil, err
	}
	rollback, _, err := container(files, rc, password, id+"-recovery", deployment, rollbackAction)
	if err != nil {
		return p, nil, nil, err
	}
	p.CandidateSHA256, p.RollbackSHA256, p.Paths = Digest(candidate), Digest(rollback), paths
	return p, candidate, rollback, p.Validate()
}

func readBundle(dir string) (map[string][]byte, error) {
	manifest, err := os.ReadFile(filepath.Join(dir, "SHA256SUMS"))
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	for _, line := range strings.Split(strings.TrimSpace(string(manifest)), "\n") {
		parts := strings.Fields(line)
		if len(parts) != 2 || len(parts[0]) != 64 || !safeFile.MatchString(parts[1]) || parts[1] == "." || parts[1] == ".." {
			return nil, errors.New("unsupported bundle manifest entry")
		}
		name := parts[1]
		if _, exists := files[name]; exists {
			return nil, errors.New("duplicate bundle entry")
		}
		info, err := os.Lstat(filepath.Join(dir, name))
		if err != nil || !info.Mode().IsRegular() || info.Size() > 64<<20 {
			return nil, fmt.Errorf("invalid bundle file: %s", name)
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		if Digest(data) != parts[0] {
			return nil, fmt.Errorf("checksum mismatch: %s", name)
		}
		files[name] = data
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	if len(entries) != len(files)+1 {
		return nil, errors.New("bundle has unlisted files")
	}
	bin := files["ufi-tools-u60pro"]
	if len(bin) < 20 || !bytes.Equal(bin[:4], []byte{0x7f, 'E', 'L', 'F'}) || bin[4] != 2 || bin[5] != 1 || bin[18] != 183 || bin[19] != 0 {
		return nil, errors.New("bundle needs a Linux AArch64 ELF")
	}
	if !bytes.Contains(files["service.sh"], []byte("lan-bootstrap")) {
		return nil, errors.New("service script lacks LAN bootstrap support; rebuild the bundle")
	}
	return files, nil
}

func PatchBoot(before []byte, line string) ([]byte, error) {
	if bytes.Contains(before, []byte(".lan-stage-")) {
		return nil, errors.New("an earlier LAN bootstrap must be resolved first")
	}
	lines := strings.SplitAfter(string(before), "\n")
	exitAt := -1
	ownedAt := -1
	for i, s := range lines {
		t := strings.TrimSpace(s)
		if t == "sh "+Root+"/service.sh start" {
			if ownedAt != -1 {
				return nil, errors.New("duplicate candidate startup entries")
			}
			ownedAt = i
		}
		if t == "exit 0" {
			if exitAt != -1 {
				return nil, errors.New("ambiguous rc.local exits")
			}
			exitAt = i
		} else if strings.HasPrefix(t, "exit ") || t == "exit" {
			return nil, errors.New("unsupported rc.local exit")
		}
		if exitAt >= 0 && i > exitAt && t != "" && !strings.HasPrefix(t, "#") {
			return nil, errors.New("commands follow rc.local exit")
		}
	}
	if ownedAt >= 0 {
		lines[ownedAt] = line + "\n"
		return []byte(strings.Join(lines, "")), nil
	}
	if exitAt >= 0 {
		lines[exitAt] = line + "\n" + lines[exitAt]
		return []byte(strings.Join(lines, "")), nil
	}
	result := append([]byte(nil), before...)
	if len(result) > 0 && result[len(result)-1] != '\n' {
		result = append(result, '\n')
	}
	return append(result, []byte(line+"\n")...), nil
}

func container(original map[string][]byte, rc ztebackup.File, password []byte, stageID, deployment, action string) ([]byte, []string, error) {
	stage := Root + ".lan-stage-" + stageID
	line := "UFI_INSTALL_ROOT=" + Root + " UFI_RC_LOCAL=/etc/rc.local sh " + stage + "/service.sh lan-bootstrap " + stage + " >>" + stage + ".log 2>&1"
	after, err := PatchBoot(rc.Data, line)
	if err != nil {
		return nil, nil, err
	}
	files := map[string][]byte{}
	for name, data := range original {
		files[name] = data
	}
	files["lan-rc-before"] = rc.Data
	files["lan-rc-after"] = after
	files["lan-plan-id"] = []byte(deployment + "\n")
	files["lan-action"] = []byte(action + "\n")
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var sums strings.Builder
	archive := []ztebackup.File{{Path: RCPath, Mode: rc.Mode, Data: after}}
	for _, name := range names {
		fmt.Fprintf(&sums, "%s  %s\n", Digest(files[name]), name)
		mode := int64(0o600)
		if name == "service.sh" || name == "ufi-tools-u60pro" || name == "verify-release" {
			mode = 0o755
		}
		archive = append(archive, ztebackup.File{Path: "data" + stage + "/" + name, Mode: mode, Data: files[name]})
	}
	archive = append(archive, ztebackup.File{Path: "data" + stage + "/SHA256SUMS", Mode: 0o600, Data: []byte(sums.String())})
	data, err := (ztebackup.Builder{Password: password}).Build(archive)
	paths := make([]string, 0, len(archive))
	for _, file := range archive {
		paths = append(paths, file.Path)
	}
	sort.Strings(paths)
	return data, paths, err
}

func Save(dir string, p Plan, candidate, rollback []byte) error {
	if err := os.Mkdir(dir, 0o700); err != nil {
		return err
	}
	for name, data := range map[string][]byte{"candidate.bin": candidate, "rollback.bin": rollback} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			return err
		}
	}
	if p.LoginToken != "" {
		if err := os.WriteFile(filepath.Join(dir, "candidate-token"), []byte(p.LoginToken+"\n"), 0o600); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "plan.json"), append(data, '\n'), 0o600)
}

func Load(dir string) (Plan, []byte, []byte, error) {
	var p Plan
	data, err := os.ReadFile(filepath.Join(dir, "plan.json"))
	if err != nil {
		return p, nil, nil, err
	}
	if err = json.Unmarshal(data, &p); err != nil {
		return p, nil, nil, err
	}
	if err = p.Validate(); err != nil {
		return p, nil, nil, err
	}
	candidate, err := os.ReadFile(filepath.Join(dir, "candidate.bin"))
	if err != nil {
		return p, nil, nil, err
	}
	rollback, err := os.ReadFile(filepath.Join(dir, "rollback.bin"))
	if err != nil {
		return p, nil, nil, err
	}
	if Digest(candidate) != p.CandidateSHA256 || Digest(rollback) != p.RollbackSHA256 {
		return p, nil, nil, errors.New("LAN plan artifact checksum mismatch")
	}
	return p, candidate, rollback, nil
}

// ValidateArtifacts rechecks the encrypted payload, not just the editable JSON
// manifest, before any authenticated device operation is attempted.
func ValidateArtifacts(p Plan, candidate, rollback, password []byte) error {
	for i, data := range [][]byte{candidate, rollback} {
		stageID := p.ID
		action := "install"
		if p.Action == "uninstall" {
			action = "remove"
		}
		if i == 1 {
			stageID += "-recovery"
			action = "remove"
			if p.Action == "uninstall" {
				action = "restore"
			}
		}
		prefix := "data" + Root + ".lan-stage-" + stageID + "/"
		report, err := ztebackup.Inspect(data, password)
		if err != nil {
			return err
		}
		if len(report.Symlinks) > 0 {
			return errors.New("LAN payload must contain regular files only")
		}
		for _, name := range report.Paths {
			if name != RCPath && !strings.HasPrefix(name, prefix) {
				return errors.New("LAN payload writes outside its isolated staging tree")
			}
		}
		if i == 0 && strings.Join(report.Paths, "\n") != strings.Join(p.Paths, "\n") {
			return errors.New("LAN path preview differs from actual container")
		}
		boot, err := ztebackup.ReadFile(data, password, RCPath)
		if err != nil || boot.Mode != p.BaselineMode {
			return errors.New("LAN boot mode differs from baseline")
		}
		before, err := ztebackup.ReadFile(data, password, prefix+"lan-rc-before")
		if err != nil || Digest(before.Data) != p.BaselineSHA256 {
			return errors.New("LAN boot preimage differs from plan")
		}
		after, err := ztebackup.ReadFile(data, password, prefix+"lan-rc-after")
		if err != nil || !bytes.Equal(after.Data, boot.Data) {
			return errors.New("LAN boot postimage differs from container")
		}
		verb, err := ztebackup.ReadFile(data, password, prefix+"lan-action")
		if err != nil || string(verb.Data) != action+"\n" {
			return errors.New("LAN action differs from plan")
		}
		id, err := ztebackup.ReadFile(data, password, prefix+"lan-plan-id")
		if err != nil || string(id.Data) != p.DeploymentID+"\n" {
			return errors.New("LAN receipt differs from plan")
		}
		binary, err := ztebackup.ReadFile(data, password, prefix+"ufi-tools-u60pro")
		if err != nil || Digest(binary.Data) != p.BinarySHA256 {
			return errors.New("LAN binary differs from plan")
		}
	}
	return nil
}
