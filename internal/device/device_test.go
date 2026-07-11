// SPDX-License-Identifier: AGPL-3.0-or-later

package device

import (
	"context"
	"testing"
)

func TestSimulatorGoFormRoundTrip(t *testing.T) {
	d := NewSimulator()
	if err := d.GoFormSet(context.Background(), "apn", "internet"); err != nil {
		t.Fatal(err)
	}
	got, err := d.GoFormGet(context.Background(), "apn")
	if err != nil || got != "internet" {
		t.Fatalf("got %q, %v", got, err)
	}
}
