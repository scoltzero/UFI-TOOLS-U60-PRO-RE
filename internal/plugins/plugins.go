// SPDX-License-Identifier: AGPL-3.0-or-later

package plugins

import (
	"errors"
	"regexp"
	"strings"
)

const MaxDocumentSize = 1145 * 1024

type Format string

const (
	FormatScript   Format = "script"
	FormatComment  Format = "comment-script"
	FormatKano     Format = "kano-markers"
	FormatDisabled Format = "kano-disabled"
)

type Document struct {
	Format   Format            `json:"format"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Source   string            `json:"source"`
	Disabled bool              `json:"disabled"`
}

var markerLine = regexp.MustCompile(`(?m)^\s*//\s*@?([A-Za-z][A-Za-z0-9_-]*)\s*[:=]\s*(.*?)\s*$`)

func Parse(source string) (Document, error) {
	if len(source) > MaxDocumentSize {
		return Document{}, errors.New("plugin document is too large")
	}
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return Document{}, errors.New("plugin document is empty")
	}

	doc := Document{Source: source, Metadata: map[string]string{}}
	switch {
	case strings.Contains(trimmed, "kano_disabled"):
		doc.Format, doc.Disabled = FormatDisabled, true
	case strings.Contains(trimmed, "KANO_PLUGIN_START") && strings.Contains(trimmed, "KANO_PLUGIN_END"):
		doc.Format = FormatKano
	case strings.HasPrefix(trimmed, "//<script"):
		doc.Format = FormatComment
	case strings.Contains(strings.ToLower(trimmed), "<script"):
		doc.Format = FormatScript
	default:
		return Document{}, errors.New("unrecognized plugin format")
	}
	for _, match := range markerLine.FindAllStringSubmatch(source, -1) {
		doc.Metadata[strings.ToLower(match[1])] = strings.TrimSpace(match[2])
	}
	return doc, nil
}
