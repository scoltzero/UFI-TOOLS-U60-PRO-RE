// SPDX-License-Identifier: AGPL-3.0-or-later

package pluginaudit

import (
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/catalog"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/plugins"
)

type Finding struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Version  string         `json:"version"`
	Format   plugins.Format `json:"format"`
	URLs     []string       `json:"urls,omitempty"`
	Hosts    []string       `json:"hosts,omitempty"`
	APIs     []string       `json:"api_paths,omitempty"`
	Warnings []string       `json:"warnings,omitempty"`
}

var (
	urlPattern = regexp.MustCompile(`https?://[^\s"'<>\\)]+`)
	apiPattern = regexp.MustCompile(`/api/[A-Za-z0-9_./?=&%{}:-]+`)
	riskTerms  = map[string]string{
		"eval(": "dynamic JavaScript evaluation", "new Function": "dynamic JavaScript construction",
		"/api/root_shell": "root shell API", "/api/user_shell": "user shell API",
		"rm -rf": "recursive deletion command", "curl ": "external download command", "wget ": "external download command",
	}
)

func Catalog(root *catalog.Catalog) ([]Finding, error) {
	items := root.List()
	findings := make([]Finding, 0, len(items))
	for _, item := range items {
		_, data, err := root.Read(item.ID)
		if err != nil {
			return nil, err
		}
		doc, err := plugins.Parse(string(data))
		if err != nil {
			return nil, err
		}
		finding := Finding{ID: item.ID, Name: item.Name, Version: item.Version, Format: doc.Format}
		finding.URLs = unique(urlPattern.FindAllString(doc.Source, -1))
		finding.APIs = unique(apiPattern.FindAllString(doc.Source, -1))
		hosts := make([]string, 0, len(finding.URLs))
		for _, raw := range finding.URLs {
			if parsed, err := url.Parse(raw); err == nil && parsed.Hostname() != "" {
				hosts = append(hosts, strings.ToLower(parsed.Hostname()))
			}
		}
		finding.Hosts = unique(hosts)
		for term, warning := range riskTerms {
			if strings.Contains(doc.Source, term) {
				finding.Warnings = append(finding.Warnings, warning)
			}
		}
		sort.Strings(finding.Warnings)
		findings = append(findings, finding)
	}
	return findings, nil
}

func unique(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimRight(value, ".,;]")
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}
