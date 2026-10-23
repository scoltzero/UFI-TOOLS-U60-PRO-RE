// SPDX-License-Identifier: AGPL-3.0-or-later
package plugins

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var managedID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)

func managedMarkers(id string) (string, string, error) {
	if !managedID.MatchString(id) {
		return "", "", errors.New("invalid plugin id")
	}
	name := "ufi-store:" + id
	return "<!-- [KANO_PLUGIN_START] " + name + " -->", "<!-- [KANO_PLUGIN_END] " + name + " -->", nil
}

// Remove only this catalog entry's marked fragment. User fragments and other
// catalog entries remain byte-for-byte intact. Never guess past a broken marker.
func RemoveManaged(source, id string) (string, error) {
	start, end, err := managedMarkers(id)
	if err != nil {
		return "", err
	}
	for {
		pos := strings.Index(source, start)
		if pos < 0 {
			return source, nil
		}
		tail := source[pos+len(start):]
		last := strings.Index(tail, end)
		if last < 0 || strings.Contains(tail[:last], start) {
			return "", errors.New("incomplete plugin markers")
		}
		source = source[:pos] + tail[last+len(end):]
	}
}

func ManagedEnabled(source, id string) bool {
	start, end, err := managedMarkers(id)
	if err != nil {
		return false
	}
	pos := strings.Index(source, start)
	if pos < 0 {
		return false
	}
	tail := source[pos+len(start):]
	last := strings.Index(tail, end)
	if last < 0 {
		return false
	}
	return !strings.HasPrefix(strings.TrimSpace(tail[:last]), "<!-- [kano_disabled]")
}

func UpsertManaged(source, id, body string) (string, error) {
	if _, err := Parse(body); err != nil {
		return "", err
	}
	start, end, err := managedMarkers(id)
	if err != nil {
		return "", err
	}
	position := strings.Index(source, start)
	exists := position >= 0
	disabled := exists && !ManagedEnabled(source, id)
	next, err := RemoveManaged(source, id)
	if err != nil {
		return "", err
	}
	if disabled && !strings.HasPrefix(strings.TrimSpace(body), "<!-- [kano_disabled]") {
		body = "<!-- [kano_disabled]\n" + body + "\n[kano_disabled] -->"
	}
	block := fmt.Sprintf("%s\n%s\n%s", start, body, end)
	if exists {
		next = next[:position] + block + next[position:]
	} else {
		next += "\n" + block + "\n"
	}
	if len(next) > MaxDocumentSize {
		return "", errors.New("combined plugin document is too large")
	}
	return next, nil
}
