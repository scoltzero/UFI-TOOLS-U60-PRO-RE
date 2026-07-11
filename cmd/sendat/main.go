// SPDX-License-Identifier: MIT
// Adapted from the MIT-licensed kanoqwq/UFI-TOOLS send_at.go helper.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var hexWord = regexp.MustCompile(`\b[0-9a-fA-F]{8}\b`)

func main() {
	command := flag.String("c", "", "AT command")
	slot := flag.Int("n", 0, "SIM slot (0 or 1)")
	flag.Parse()
	if *command == "" || !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(*command)), "AT") || len(*command) > 512 || (*slot != 0 && *slot != 1) {
		fmt.Fprintln(os.Stderr, "usage: sendat -n <0|1> -c <AT command>")
		os.Exit(2)
	}
	service, args := serviceArgs(apiLevel(), *slot, strings.TrimSpace(*command))
	output, err := exec.Command(service, args...).CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "service call failed: %v\n%s", err, output)
		os.Exit(1)
	}
	fmt.Println(parseServiceOutput(string(output)))
}

func apiLevel() int {
	output, err := exec.Command("getprop", "ro.build.version.sdk").Output()
	if err != nil {
		return 35
	}
	level, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return 35
	}
	return level
}

func serviceArgs(level, slot int, command string) (string, []string) {
	if level > 33 {
		return "/system/bin/service", []string{"call", "vendor.sprd.hardware.tool.IToolControl/default", "3", "i32", strconv.Itoa(slot), "s16", command}
	}
	return "/system/bin/service", []string{"call", "vendor.sprd.hardware.log.ILogControl/default", "1", "s16", "miscserver", "s16", "sendAt " + strconv.Itoa(slot) + " " + command}
}

func parseServiceOutput(input string) string {
	var output bytes.Buffer
	for _, word := range hexWord.FindAllString(input, -1) {
		for _, pair := range [][2]int{{6, 4}, {2, 0}} {
			low, err1 := strconv.ParseUint(word[pair[0]:pair[0]+2], 16, 8)
			high, err2 := strconv.ParseUint(word[pair[1]:pair[1]+2], 16, 8)
			if err1 == nil && err2 == nil {
				r := rune(uint16(high)<<8 | uint16(low))
				if r >= 32 && r != 127 {
					output.WriteRune(r)
				}
			}
		}
	}
	return strings.TrimSpace(output.String())
}
