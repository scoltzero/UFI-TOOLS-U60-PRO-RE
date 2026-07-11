// SPDX-License-Identifier: AGPL-3.0-or-later

package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/catalog"
)

type installedPlugin struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
	Size    int64  `json:"size,omitempty"`
}

func (h *handler) pluginList(w http.ResponseWriter, r *http.Request) {
	rows, err := h.deps.Store.ListPrefix(r.Context(), "kv", "pluginmeta.")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "list installed plugins"})
		return
	}
	items := make([]installedPlugin, 0, len(rows))
	for _, row := range rows {
		var item installedPlugin
		if json.Unmarshal([]byte(row.Value), &item) == nil {
			items = append(items, item)
		}
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: items})
}

func (h *handler) pluginInstallAlias(w http.ResponseWriter, r *http.Request) {
	if h.deps.Catalog == nil {
		writeJSON(w, http.StatusConflict, envelope{Code: 409, Message: "no plugin catalog configured"})
		return
	}
	var request struct {
		ID       string `json:"id"`
		PluginID string `json:"plugin_id"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, envelope{Code: 400, Message: "invalid plugin request"})
		return
	}
	id := request.ID
	if id == "" {
		id = request.PluginID
	}
	item, data, err := h.deps.Catalog.Download(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, envelope{Code: 502, Message: "plugin download or integrity check failed"})
		return
	}
	metadata := installedPlugin{ID: item.ID, Name: item.Name, Version: item.Version, SHA256: item.SHA256, Size: item.Size}
	encoded, _ := json.Marshal(metadata)
	if err := h.deps.Store.Set(r.Context(), "kv", "plugin."+item.ID, string(data)); err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "save plugin"})
		return
	}
	if err := h.deps.Store.Set(r.Context(), "kv", "pluginmeta."+item.ID, string(encoded)); err != nil {
		_ = h.deps.Store.Delete(r.Context(), "kv", "plugin."+item.ID)
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "save plugin metadata"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: item})
}

func (h *handler) pluginUninstall(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ID       string `json:"id"`
		PluginID string `json:"plugin_id"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, envelope{Code: 400, Message: "invalid plugin request"})
		return
	}
	id := request.ID
	if id == "" {
		id = request.PluginID
	}
	if id == "" || strings.ContainsAny(id, "/\\") {
		writeJSON(w, http.StatusBadRequest, envelope{Code: 400, Message: "plugin id is required"})
		return
	}
	if err := h.deps.Store.Delete(r.Context(), "kv", "plugin."+id); err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "remove plugin"})
		return
	}
	_ = h.deps.Store.Delete(r.Context(), "kv", "pluginmeta."+id)
	writeJSON(w, http.StatusOK, envelope{Code: 0})
}

func (h *handler) pluginUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if h.deps.Catalog == nil {
		writeJSON(w, http.StatusOK, envelope{Code: 0, Data: []any{}})
		return
	}
	catalogItems, err := h.deps.Catalog.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, envelope{Code: 502, Message: "plugin catalog unavailable"})
		return
	}
	installedRows, err := h.deps.Store.ListPrefix(r.Context(), "kv", "pluginmeta.")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "read installed plugins"})
		return
	}
	installed := map[string]installedPlugin{}
	for _, row := range installedRows {
		var item installedPlugin
		if json.Unmarshal([]byte(row.Value), &item) == nil {
			installed[item.ID] = item
		}
	}
	updates := []catalog.Plugin{}
	for _, item := range catalogItems {
		if current, ok := installed[item.ID]; ok && current.SHA256 != item.SHA256 {
			updates = append(updates, item)
		}
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: updates})
}

func (h *handler) emptyPluginEvents(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: []any{}})
}
