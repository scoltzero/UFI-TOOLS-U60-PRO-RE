// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/catalog"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/pluginaudit"
)

func main() {
	root := flag.String("root", "./plugin-store", "verified plugin snapshot directory")
	output := flag.String("output", "", "optional JSON report path")
	flag.Parse()
	store, err := catalog.Load(*root)
	fatal(err)
	findings, err := pluginaudit.Catalog(store)
	fatal(err)
	encoded, err := json.MarshalIndent(map[string]any{"plugins": findings, "count": len(findings)}, "", "  ")
	fatal(err)
	encoded = append(encoded, '\n')
	if *output == "" {
		_, err = os.Stdout.Write(encoded)
	} else {
		err = os.WriteFile(*output, encoded, 0o600)
	}
	fatal(err)
}

func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
