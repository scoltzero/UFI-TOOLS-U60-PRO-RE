// SPDX-License-Identifier: AGPL-3.0-or-later
package device

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"
)

const accessRulePrefix = "ufi_re_block_"

func accessRuleID(mac string) string {
	return accessRulePrefix + strings.ReplaceAll(strings.ToLower(mac), ":", "")
}
func ownedAccessRule(id string, v map[string]any) bool {
	return strings.HasPrefix(id, accessRulePrefix) && textValue(v["ufi_tools_re"]) == "1"
}
func (u *U60UBUS) firewallConfig(ctx context.Context) (map[string]map[string]any, error) {
	data, err := u.call(ctx, "uci", "get", map[string]any{"config": "firewall"})
	if err != nil {
		return nil, err
	}
	values, ok := data["values"].(map[string]any)
	if !ok {
		return nil, errors.New("unrecognized firewall configuration")
	}
	out := map[string]map[string]any{}
	for id, value := range values {
		if v, ok := value.(map[string]any); ok {
			out[id] = v
		}
	}
	return out, nil
}
func (u *U60UBUS) managedAccess(ctx context.Context) ([]string, []string, error) {
	rules, err := u.firewallConfig(ctx)
	if err != nil {
		return nil, nil, err
	}
	ids := []string{}
	for id, v := range rules {
		if ownedAccessRule(id, v) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	macs, names := []string{}, []string{}
	for _, id := range ids {
		v := rules[id]
		macs = append(macs, strings.ToUpper(textValue(v["src_mac"])))
		names = append(names, textValue(v["ufi_name"]))
	}
	return macs, names, nil
}
func (u *U60UBUS) setManagedAccess(ctx context.Context, v map[string]string) error {
	if u.local == nil {
		return ErrUnsupported
	}
	if v["WhiteMacList"] != "" || v["AclMode"] != "" && v["AclMode"] != "0" && v["AclMode"] != "1" {
		return invalid("only the candidate's internet block list is supported")
	}
	desired := map[string]string{}
	names := strings.Split(v["BlackNameList"], ";")
	if v["AclMode"] != "0" {
		for i, raw := range strings.Split(v["BlackMacList"], ";") {
			if strings.TrimSpace(raw) == "" {
				continue
			}
			mac, e := net.ParseMAC(strings.TrimSpace(raw))
			if e != nil || len(mac) != 6 {
				return invalid("block-list MAC address")
			}
			name := ""
			if i < len(names) {
				name = names[i]
			}
			if len(name) > 128 || strings.ContainsAny(name, ";\x00\r\n") {
				return invalid("device label")
			}
			desired[mac.String()] = name
		}
	}
	if len(desired) > 256 {
		return invalid("block list is too large")
	}
	before, err := u.firewallConfig(ctx)
	if err != nil {
		return err
	}
	source := ""
	for _, zone := range before {
		if textValue(zone[".type"]) != "zone" {
			continue
		}
		networks := textValue(zone["network"])
		hasLAN := false
		for _, network := range strings.Fields(strings.NewReplacer("[", " ", "]", " ").Replace(networks)) {
			if network == "lan" {
				hasLAN = true
			}
		}
		if textValue(zone["name"]) == "lan" || hasLAN {
			source = textValue(zone["name"])
			break
		}
	}
	if source == "" {
		return errors.New("LAN firewall zone is unavailable")
	}
	for mac := range desired {
		if existing, ok := before[accessRuleID(mac)]; ok && !ownedAccessRule(accessRuleID(mac), existing) {
			return invalid("firewall section is owned by another configuration")
		}
	}
	// Only our marked sections are changed. Existing parental controls, firewall
	// rules, and user-created sections remain untouched.
	apply := func(target map[string]map[string]any) error {
		current, e := u.firewallConfig(ctx)
		if e != nil {
			return e
		}
		for id, rule := range current {
			if ownedAccessRule(id, rule) {
				if _, e = u.call(ctx, "uci", "delete", map[string]any{"config": "firewall", "section": id}); e != nil {
					return e
				}
			}
		}
		for id, values := range target {
			if _, e = u.call(ctx, "uci", "add", map[string]any{"config": "firewall", "type": "rule", "name": id, "values": values}); e != nil {
				return e
			}
		}
		if _, e = u.call(ctx, "uci", "commit", map[string]any{"config": "firewall"}); e != nil {
			return e
		}
		_, e = u.local.Runner.Run(ctx, 10*time.Second, "/etc/init.d/firewall", "reload")
		return e
	}
	saved := map[string]map[string]any{}
	for id, rule := range before {
		if ownedAccessRule(id, rule) {
			values := map[string]any{}
			for k, x := range rule {
				if !strings.HasPrefix(k, ".") {
					values[k] = x
				}
			}
			saved[id] = values
		}
	}
	next := map[string]map[string]any{}
	for mac, name := range desired {
		next[accessRuleID(mac)] = map[string]any{"name": "UFI candidate internet restriction", "ufi_tools_re": "1", "ufi_name": name, "src": source, "dest": "*", "proto": "all", "src_mac": mac, "target": "REJECT", "enabled": "1"}
	}
	if err = apply(next); err != nil {
		if restoreErr := apply(saved); restoreErr != nil {
			return fmt.Errorf("%w; access-rule rollback incomplete: %v", err, restoreErr)
		}
		return err
	}
	return nil
}
