// SPDX-License-Identifier: AGPL-3.0-or-later
package device

import (
	"context"
	"errors"
	"fmt"
)

// SelectAPN leaves the old selection intact when switching mode fails.
func SelectAPN(ctx context.Context, rpc UBUSDevice, mode, id string) error {
	if mode == "auto" {
		_, err := rpc.Call(ctx, "zwrt_apn_object", "set_apn_mode", map[string]any{"apn_mode": 0})
		return err
	}
	old, err := rpc.Call(ctx, "zwrt_apn_object", "get_enabled_manu_apn_id", map[string]any{})
	if err != nil {
		return err
	}
	if _, err = rpc.Call(ctx, "zwrt_apn_object", "enable_manu_apn_id", map[string]any{"profileId": id}); err != nil {
		return err
	}
	if _, err = rpc.Call(ctx, "zwrt_apn_object", "set_apn_mode", map[string]any{"apn_mode": 1}); err != nil {
		previous := textValue(old["profileId"])
		if previous != "" && previous != id {
			if _, restore := rpc.Call(ctx, "zwrt_apn_object", "enable_manu_apn_id", map[string]any{"profileId": previous}); restore != nil {
				return errors.Join(err, fmt.Errorf("restore APN selection: %w", restore))
			}
		}
		return err
	}
	return nil
}
