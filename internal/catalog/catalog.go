// SPDX-License-Identifier: AGPL-3.0-or-later

package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/plugins"
)

type Plugin struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
	Author      string `json:"author,omitempty"`
	File        string `json:"file"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
}

type Manifest struct {
	Version int      `json:"version"`
	Plugins []Plugin `json:"plugins"`
}

type Catalog struct {
	root    string
	plugins map[string]Plugin
}

var safeID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)

func Load(root string) (*Catalog, error) {
	manifestData, err := os.ReadFile(filepath.Join(root, "catalog.json"))
	if err != nil {
		return nil, fmt.Errorf("read catalog: %w", err)
	}
	var manifest Manifest
	decoder := json.NewDecoder(strings.NewReader(string(manifestData)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decode catalog: %w", err)
	}
	if manifest.Version != 1 {
		return nil, fmt.Errorf("unsupported catalog version %d", manifest.Version)
	}
	catalog := &Catalog{root: root, plugins: make(map[string]Plugin, len(manifest.Plugins))}
	for _, item := range manifest.Plugins {
		if !safeID.MatchString(item.ID) || item.Name == "" || item.Version == "" || filepath.Base(item.File) != item.File {
			return nil, fmt.Errorf("invalid plugin entry %q", item.ID)
		}
		if _, exists := catalog.plugins[item.ID]; exists {
			return nil, fmt.Errorf("duplicate plugin id %q", item.ID)
		}
		data, err := os.ReadFile(filepath.Join(root, item.File))
		if err != nil {
			return nil, fmt.Errorf("read plugin %q: %w", item.ID, err)
		}
		if _, err := plugins.Parse(string(data)); err != nil {
			return nil, fmt.Errorf("validate plugin %q: %w", item.ID, err)
		}
		digest := sha256.Sum256(data)
		actual := hex.EncodeToString(digest[:])
		if item.SHA256 == "" {
			item.SHA256 = actual
		}
		if !strings.EqualFold(item.SHA256, actual) {
			return nil, fmt.Errorf("plugin %q digest mismatch", item.ID)
		}
		if item.Size == 0 {
			item.Size = int64(len(data))
		}
		if item.Size != int64(len(data)) {
			return nil, fmt.Errorf("plugin %q size mismatch", item.ID)
		}
		catalog.plugins[item.ID] = item
	}
	return catalog, nil
}

func (c *Catalog) List() []Plugin {
	items := make([]Plugin, 0, len(c.plugins))
	for _, item := range c.plugins {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items
}

func (c *Catalog) Read(id string) (Plugin, []byte, error) {
	if !safeID.MatchString(id) {
		return Plugin{}, nil, errors.New("invalid plugin id")
	}
	item, ok := c.plugins[id]
	if !ok {
		return Plugin{}, nil, os.ErrNotExist
	}
	data, err := os.ReadFile(filepath.Join(c.root, item.File))
	if err != nil {
		return Plugin{}, nil, err
	}
	digest := sha256.Sum256(data)
	if !strings.EqualFold(hex.EncodeToString(digest[:]), item.SHA256) {
		return Plugin{}, nil, errors.New("plugin changed after catalog load")
	}
	return item, data, nil
}
