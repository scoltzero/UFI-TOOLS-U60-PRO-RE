// SPDX-License-Identifier: AGPL-3.0-or-later
package device

import (
	"context"
	"strings"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/u60"
)

// NewU60Local uses the device's local UBUS socket, without collecting router
// credentials. It must only be selected when the candidate runs on the device.
func NewU60Local(allowWrites bool) *U60UBUS {
	return &U60UBUS{local: &u60.UBUS{Runner: u60.LocalRunner{}}, metrics: &u60.Telemetry{}, allowWrites: allowWrites}
}

func readOnlyUBUS(procedure string) bool {
	p := strings.ToLower(procedure)
	return strings.HasPrefix(p, "get") || strings.HasPrefix(p, "list") || strings.Contains(p, "_get_") || p == "status" || p == "report"
}

func (u *U60UBUS) Telemetry(context.Context) (map[string]any, error) {
	if u.metrics != nil {
		return u.metrics.Read(), nil
	}
	// An HTTP backend can run on another machine. Do not substitute that
	// machine's CPU, battery or filesystem for the remote device's telemetry.
	return map[string]any{}, nil
}
