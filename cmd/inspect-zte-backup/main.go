// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/ztebackup"
)

func main() {
	input := flag.String("input", "", "authorized U60 backup file")
	passwordFile := flag.String("password-file", "", "file containing the backup password")
	imeiFile := flag.String("imei-file", "", "file containing the matching device IMEI")
	flag.Parse()
	if *input == "" || (*passwordFile == "" && *imeiFile == "") {
		fatal(fmt.Errorf("input and password-file or imei-file are required"))
	}
	data, err := os.ReadFile(*input)
	fatal(err)
	var password []byte
	if *passwordFile != "" {
		password, err = os.ReadFile(*passwordFile)
		fatal(err)
		password = []byte(strings.TrimRight(string(password), "\r\n"))
	} else {
		imei, readErr := os.ReadFile(*imeiFile)
		fatal(readErr)
		password = ztebackup.DevicePassword(strings.TrimRight(string(imei), "\r\n"))
	}
	report, err := ztebackup.Inspect(data, password)
	fatal(err)
	encoded, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(encoded))
}

func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
