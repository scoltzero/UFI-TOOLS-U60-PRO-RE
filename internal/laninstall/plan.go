// SPDX-License-Identifier: AGPL-3.0-or-later
package laninstall

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/lanbundle"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/ztebackup"
)

type PlanOptions struct {
	Plan                                              lanbundle.Plan
	Candidate, Rollback, BackupPassword               []byte
	RouterPassword, RunDir, CandidateURL, OriginalURL string
	Execute, Acknowledged                             bool
	Timeout, Interval                                 time.Duration
}
type PlanResult struct {
	PlanID                  string `json:"plan_id"`
	Action                  string `json:"action"`
	DryRun                  bool   `json:"dry_run"`
	Step                    string `json:"step"`
	BaselineMatched         bool   `json:"baseline_matched"`
	RestoreRequested        bool   `json:"restore_requested"`
	RestoreAcknowledged     bool   `json:"restore_acknowledged"`
	OutcomeObserved         bool   `json:"outcome_observed"`
	RollbackRequested       bool   `json:"rollback_requested"`
	RollbackOutcomeObserved bool   `json:"rollback_outcome_observed"`
}
type healthObservation struct {
	reachable, healthy   bool
	instance, deployment string
}

func (c *Client) validateHealthTarget(target string) error {
	u, err := url.Parse(target)
	if err != nil || u.Hostname() != c.base.Hostname() || u.Scheme != c.base.Scheme || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path == "" {
		return errors.New("health URL must use the same device host and scheme, without credentials/query/fragment")
	}
	return nil
}

func (c *Client) probe(ctx context.Context, target string) healthObservation {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return healthObservation{}
	}
	response, err := c.http.Do(req)
	if err != nil {
		return healthObservation{}
	}
	defer response.Body.Close()
	result := healthObservation{reachable: true, healthy: response.StatusCode == 200}
	var body struct {
		Data struct {
			Instance   string `json:"instance"`
			Deployment string `json:"deployment"`
		} `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&body) == nil {
		result.instance = body.Data.Instance
		result.deployment = body.Data.Deployment
	}
	return result
}

// RunPlan verifies the current boot baseline before uploading an isolated
// operation. OutcomeObserved proves service state only, not partition recovery.
func (c *Client) RunPlan(ctx context.Context, o PlanOptions) (result PlanResult, err error) {
	result = PlanResult{PlanID: o.Plan.ID, Action: o.Plan.Action, DryRun: !o.Execute, Step: "validated-local-plan"}
	if err = o.Plan.Validate(); err != nil {
		return result, err
	}
	if len(o.Candidate) == 0 || len(o.Candidate) > maxBackupSize || len(o.Rollback) == 0 || len(o.Rollback) > maxBackupSize || lanbundle.Digest(o.Candidate) != o.Plan.CandidateSHA256 || lanbundle.Digest(o.Rollback) != o.Plan.RollbackSHA256 {
		return result, errors.New("invalid LAN plan artifacts")
	}
	for _, target := range []string{o.CandidateURL, o.OriginalURL} {
		if err = c.validateHealthTarget(target); err != nil {
			return result, err
		}
	}
	if o.CandidateURL == o.OriginalURL {
		return result, errors.New("candidate and original health targets must differ")
	}
	if !o.Execute {
		return result, nil
	}
	if !o.Acknowledged || o.RouterPassword == "" || len(o.BackupPassword) == 0 || o.RunDir == "" {
		return result, errors.New("execution requires reboot acknowledgement, router password, backup key and a new run directory")
	}
	if err = lanbundle.ValidateArtifacts(o.Plan, o.Candidate, o.Rollback, o.BackupPassword); err != nil {
		return result, err
	}
	if o.Timeout <= 0 {
		o.Timeout = 3 * time.Minute
	}
	if o.Interval <= 0 {
		o.Interval = 2 * time.Second
	}
	if err = os.Mkdir(o.RunDir, 0o700); err != nil {
		return result, err
	}
	defer func() {
		c.password = ""
		data, _ := json.MarshalIndent(result, "", "  ")
		if saveErr := os.WriteFile(filepath.Join(o.RunDir, "result.json"), append(data, '\n'), 0o600); saveErr != nil && err == nil {
			err = saveErr
		}
	}()
	result.Step = "login"
	if err = c.Login(ctx, o.RouterPassword); err != nil {
		return result, err
	}
	result.Step = "verify-live-baseline"
	original := c.probe(ctx, o.OriginalURL)
	before := c.probe(ctx, o.CandidateURL)
	if !original.healthy {
		return result, errors.New("original service is not healthy; no restore requested")
	}
	if o.Plan.Action == "install" && before.reachable {
		return result, errors.New("candidate port already responds; LAN supports first install only")
	}
	if o.Plan.Action == "uninstall" && (!before.healthy || before.deployment != o.Plan.DeploymentID || before.instance == "") {
		return result, errors.New("running candidate does not match the saved installation receipt")
	}
	baseline, readErr := c.Backup(ctx)
	if readErr != nil {
		return result, readErr
	}
	if err = os.WriteFile(filepath.Join(o.RunDir, "baseline.bin"), baseline, 0o600); err != nil {
		return result, err
	}
	file, readErr := ztebackup.ReadFile(baseline, o.BackupPassword, lanbundle.RCPath)
	if readErr != nil {
		return result, readErr
	}
	if lanbundle.Digest(file.Data) != o.Plan.BaselineSHA256 || file.Mode != o.Plan.BaselineMode {
		return result, errors.New("rc.local changed since preparation; prepare a new plan from a fresh backup")
	}
	result.BaselineMatched = true
	for name, data := range map[string][]byte{"baseline.bin": baseline, "rollback.bin": o.Rollback} {
		if err = os.WriteFile(filepath.Join(o.RunDir, name), data, 0o600); err != nil {
			return result, err
		}
	}
	result.Step = "upload-candidate"
	if err = c.Upload(ctx, o.Candidate); err != nil {
		return result, err
	}
	result.Step = "restore-and-observe"
	result.RestoreRequested = true
	restoreErr := c.Restore(ctx)
	result.RestoreAcknowledged = restoreErr == nil
	if c.waitPlanOutcome(ctx, o, o.Plan.Action, before.instance) {
		result.OutcomeObserved = true
		result.Step = "completed"
		return result, nil
	}
	// A dropped restore response can mean a reboot. Resolve through observation,
	// then use the pre-built inverse operation; never report a generic HTTP 200.
	result.Step = "rollback"
	rollbackBefore := c.probe(ctx, o.CandidateURL)
	recovery, cancel := context.WithTimeout(context.Background(), 2*o.Timeout)
	defer cancel()
	if err = retry(recovery, o.Interval, func() error {
		if e := c.reauthenticate(recovery); e != nil {
			return e
		}
		return c.Upload(recovery, o.Rollback)
	}); err != nil {
		return result, fmt.Errorf("outcome unconfirmed; rollback upload failed: %w", err)
	}
	result.RollbackRequested = true
	_ = c.Restore(recovery)
	inverse := "uninstall"
	if o.Plan.Action == "uninstall" {
		inverse = "install"
	}
	result.RollbackOutcomeObserved = c.waitPlanOutcome(recovery, o, inverse, rollbackBefore.instance)
	if result.RollbackOutcomeObserved {
		result.Step = "rolled-back"
		return result, errors.New("requested operation failed; inverse service outcome observed")
	}
	result.Step = "needs-inspection"
	return result, errors.New("requested operation and rollback outcome could not be verified; preserve the run directory and inspect the device")
}

func (c *Client) waitPlanOutcome(ctx context.Context, o PlanOptions, action, priorInstance string) bool {
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	seenOriginalDown := false
	for {
		original, candidate := c.probe(ctx, o.OriginalURL), c.probe(ctx, o.CandidateURL)
		if !original.healthy {
			seenOriginalDown = true
		}
		if action == "install" && original.healthy && candidate.healthy && candidate.deployment == o.Plan.DeploymentID && candidate.instance != "" && candidate.instance != priorInstance {
			return true
		}
		if action == "uninstall" && seenOriginalDown && original.healthy && !candidate.reachable {
			return true
		}
		timer := time.NewTimer(o.Interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false
		case <-timer.C:
		}
	}
}
