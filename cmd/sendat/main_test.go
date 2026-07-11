// SPDX-License-Identifier: MIT

package main

import (
	"reflect"
	"testing"
)

func TestServiceArgsByAPILevel(t *testing.T) {
	service, args := serviceArgs(35, 1, "AT+CSQ")
	if service != "/system/bin/service" || !reflect.DeepEqual(args, []string{"call", "vendor.sprd.hardware.tool.IToolControl/default", "3", "i32", "1", "s16", "AT+CSQ"}) {
		t.Fatalf("unexpected modern args: %s %#v", service, args)
	}
	_, legacy := serviceArgs(33, 0, "AT+CSQ")
	if legacy[len(legacy)-1] != "sendAt 0 AT+CSQ" {
		t.Fatalf("unexpected legacy args: %#v", legacy)
	}
}
