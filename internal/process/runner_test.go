// SPDX-License-Identifier: AGPL-3.0-or-later

package process

import (
	"context"
	"testing"
	"time"
)

func TestLocalRunnerExitAndLimit(t *testing.T) {
	runner := LocalRunner{Timeout: time.Second, MaxOutput: 4}
	result, err := runner.Run(context.Background(), "printf abcdef; exit 7")
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 7 || result.Output != "abcd" || !result.Truncated {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestLocalRunnerTimeout(t *testing.T) {
	runner := LocalRunner{Timeout: 10 * time.Millisecond}
	result, err := runner.Run(context.Background(), "sleep 1")
	if err != nil {
		t.Fatal(err)
	}
	if !result.TimedOut || result.ExitCode != -1 {
		t.Fatalf("unexpected result: %#v", result)
	}
}
