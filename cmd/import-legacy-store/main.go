// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/catalog"
)

type stringsFlag []string

func (s *stringsFlag) String() string { return fmt.Sprint([]string(*s)) }
func (s *stringsFlag) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func main() {
	baseURL := flag.String("url", "", "legacy HTTPS store origin")
	output := flag.String("output", "", "new self-hosted catalog directory")
	var allowedDownloadHosts stringsFlag
	flag.Var(&allowedDownloadHosts, "allow-download-host", "explicit HTTPS redirect host; repeat for multiple hosts")
	flag.Parse()
	if *baseURL == "" || *output == "" {
		fmt.Fprintln(os.Stderr, "url and output are required")
		os.Exit(2)
	}
	importer, err := catalog.NewLegacyImporter(*baseURL, nil, allowedDownloadHosts...)
	if err != nil {
		fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	manifest, err := importer.Import(ctx, *output)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("imported %d verified plugins into %s\n", len(manifest.Plugins), *output)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
