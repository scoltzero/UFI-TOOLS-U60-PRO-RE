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
	"strings"
	"time"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/plugins"
)

type Client struct {
	base  *url.URL
	token string
	http  *http.Client
}

func NewClient(baseURL, token string, client *http.Client) (*Client, error) {
	base, err := url.Parse(baseURL)
	if err != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") {
		return nil, errors.New("invalid catalog URL")
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	base.Path = strings.TrimSuffix(base.Path, "/")
	return &Client{base: base, token: token, http: client}, nil
}

func (c *Client) List(ctx context.Context) ([]Plugin, error) {
	var response struct {
		Version int      `json:"version"`
		Plugins []Plugin `json:"plugins"`
	}
	if err := c.getJSON(ctx, "/api/v1/plugins", &response); err != nil {
		return nil, err
	}
	if response.Version != 1 || len(response.Plugins) > 1000 {
		return nil, errors.New("unsupported or oversized catalog")
	}
	return response.Plugins, nil
}

func (c *Client) Download(ctx context.Context, id string) (Plugin, []byte, error) {
	if !safeID.MatchString(id) {
		return Plugin{}, nil, errors.New("invalid plugin id")
	}
	items, err := c.List(ctx)
	if err != nil {
		return Plugin{}, nil, err
	}
	var selected Plugin
	for _, item := range items {
		if item.ID == id {
			selected = item
			break
		}
	}
	if selected.ID == "" {
		return Plugin{}, nil, errors.New("plugin is absent from catalog")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.resolve("/api/v1/plugins/"+url.PathEscape(id)), nil)
	if err != nil {
		return Plugin{}, nil, err
	}
	c.authorize(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return Plugin{}, nil, fmt.Errorf("download plugin: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Plugin{}, nil, fmt.Errorf("plugin server returned %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, plugins.MaxDocumentSize+1))
	if err != nil {
		return Plugin{}, nil, err
	}
	if len(data) > plugins.MaxDocumentSize || int64(len(data)) != selected.Size {
		return Plugin{}, nil, errors.New("plugin size mismatch")
	}
	digest := sha256.Sum256(data)
	if !strings.EqualFold(hex.EncodeToString(digest[:]), selected.SHA256) {
		return Plugin{}, nil, errors.New("plugin digest mismatch")
	}
	if _, err := plugins.Parse(string(data)); err != nil {
		return Plugin{}, nil, fmt.Errorf("plugin format: %w", err)
	}
	return selected, data, nil
}

func (c *Client) getJSON(ctx context.Context, path string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.resolve(path), nil)
	if err != nil {
		return err
	}
	c.authorize(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("catalog request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("plugin server returned %d", resp.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 2<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func (c *Client) authorize(req *http.Request) {
	if c.token != "" {
		req.Header.Set("X-Store-Token", c.token)
	}
}

func (c *Client) resolve(path string) string {
	copy := *c.base
	copy.Path = strings.TrimSuffix(c.base.Path, "/") + path
	return copy.String()
}
