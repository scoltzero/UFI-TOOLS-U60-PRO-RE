// SPDX-License-Identifier: AGPL-3.0-or-later

package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/plugins"
)

type LegacyImporter struct {
	base             *url.URL
	client           *http.Client
	allowedDownloads map[string]bool
}

type legacyPlugin struct {
	Name       string `json:"name"`
	PublicName string `json:"public_name"`
	Size       int64  `json:"size"`
	Meta       struct {
		PluginID    string `json:"plugin_id"`
		Name        string `json:"name"`
		Version     string `json:"version"`
		Author      string `json:"author"`
		Description string `json:"description"`
	} `json:"meta"`
}

func NewLegacyImporter(baseURL string, client *http.Client, allowedDownloadHosts ...string) (*LegacyImporter, error) {
	base, err := url.Parse(baseURL)
	if err != nil || base.Host == "" || base.Scheme != "https" {
		return nil, errors.New("legacy store must be an HTTPS origin")
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	allowed := map[string]bool{}
	for _, host := range allowedDownloadHosts {
		host = strings.ToLower(strings.TrimSpace(host))
		if host == "" || strings.ContainsAny(host, "/:@") {
			return nil, fmt.Errorf("invalid allowed download host %q", host)
		}
		allowed[host] = true
	}
	clientCopy := *client
	clientCopy.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		sameOrigin := req.URL.Scheme == base.Scheme && strings.EqualFold(req.URL.Host, base.Host)
		explicitHost := req.URL.Scheme == "https" && allowed[strings.ToLower(req.URL.Hostname())]
		if !sameOrigin && !explicitHost {
			return fmt.Errorf("legacy store redirect host %q is not explicitly allowed", req.URL.Hostname())
		}
		if len(via) > 5 {
			return errors.New("too many legacy store redirects")
		}
		return nil
	}
	base.Path = strings.TrimSuffix(base.Path, "/")
	return &LegacyImporter{base: base, client: &clientCopy, allowedDownloads: allowed}, nil
}

func (i *LegacyImporter) Import(ctx context.Context, output string) (Manifest, error) {
	var listing struct {
		Code int            `json:"code"`
		Data []legacyPlugin `json:"data"`
	}
	if err := i.getJSON(ctx, "/api/v1/plugins", &listing); err != nil {
		return Manifest{}, err
	}
	if listing.Code != 0 || len(listing.Data) > 1000 {
		return Manifest{}, errors.New("legacy catalog rejected or oversized")
	}
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		return Manifest{}, errors.New("output directory already exists")
	}
	parent := filepath.Dir(output)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return Manifest{}, err
	}
	work, err := os.MkdirTemp(parent, ".ufi-plugin-import-")
	if err != nil {
		return Manifest{}, err
	}
	defer os.RemoveAll(work)
	manifest := Manifest{Version: 1, Plugins: make([]Plugin, 0, len(listing.Data))}
	seen := make(map[string]bool)
	for _, legacy := range listing.Data {
		id := legacy.Meta.PluginID
		if !safeID.MatchString(id) || seen[id] || legacy.PublicName == "" {
			return Manifest{}, fmt.Errorf("invalid or duplicate legacy plugin id %q", id)
		}
		seen[id] = true
		data, err := i.download(ctx, legacy.PublicName)
		if err != nil {
			return Manifest{}, fmt.Errorf("download %q: %w", legacy.Meta.Name, err)
		}
		if legacy.Size > 0 && int64(len(data)) != legacy.Size {
			return Manifest{}, fmt.Errorf("legacy plugin %q size mismatch", id)
		}
		if _, err := plugins.Parse(string(data)); err != nil {
			return Manifest{}, fmt.Errorf("legacy plugin %q format: %w", id, err)
		}
		extension := strings.ToLower(filepath.Ext(legacy.PublicName))
		if extension != ".js" && extension != ".txt" {
			extension = ".js"
		}
		file := id + extension
		if err := os.WriteFile(filepath.Join(work, file), data, 0o644); err != nil {
			return Manifest{}, err
		}
		digest := sha256.Sum256(data)
		manifest.Plugins = append(manifest.Plugins, Plugin{
			ID: id, Name: legacy.Meta.Name, Version: legacy.Meta.Version,
			Description: legacy.Meta.Description, Author: legacy.Meta.Author,
			File: file, SHA256: hex.EncodeToString(digest[:]), Size: int64(len(data)),
		})
	}
	sort.Slice(manifest.Plugins, func(a, b int) bool { return manifest.Plugins[a].ID < manifest.Plugins[b].ID })
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Manifest{}, err
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(filepath.Join(work, "catalog.json"), encoded, 0o644); err != nil {
		return Manifest{}, err
	}
	if err := os.Rename(work, output); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (i *LegacyImporter) download(ctx context.Context, publicName string) ([]byte, error) {
	if filepath.Base(publicName) != publicName {
		return nil, errors.New("unsafe public plugin name")
	}
	path := "/api/v1/plugins/" + url.PathEscape(publicName) + "/download"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, i.resolve(path), nil)
	if err != nil {
		return nil, err
	}
	resp, err := i.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, plugins.MaxDocumentSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > plugins.MaxDocumentSize {
		return nil, errors.New("plugin exceeds compatibility size limit")
	}
	return data, nil
}

func (i *LegacyImporter) getJSON(ctx context.Context, path string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, i.resolve(path), nil)
	if err != nil {
		return err
	}
	resp, err := i.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(target)
}

func (i *LegacyImporter) resolve(path string) string {
	copy := *i.base
	copy.Path = strings.TrimSuffix(i.base.Path, "/") + path
	return copy.String()
}
