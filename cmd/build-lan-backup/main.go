// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"fmt"
	"os"
)

// The former raw-container CLI could overwrite the original installation.
// Keep an explicit diagnostic for existing scripts instead of executing it.
func main() {
	fmt.Fprintln(os.Stderr, "Legacy raw LAN restore entry point is disabled. Use installer/install-lan.sh backup / prepare / run for an isolated, baseline-checked operation.")
	os.Exit(2)
}
