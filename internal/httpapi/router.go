// SPDX-License-Identifier: AGPL-3.0-or-later

package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/catalog"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/config"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/device"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/plugins"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/store"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/webui"
)

type Dependencies struct {
	Config  config.Config
	Store   *store.Store
	Device  device.Device
	Logger  *slog.Logger
	Version string
	Catalog *catalog.Client
}

type handler struct {
	deps Dependencies
	auth authenticator
}

type envelope struct {
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

func New(deps Dependencies) http.Handler {
	h := &handler{deps: deps, auth: authenticator{cfg: deps.Config}}
	public := http.NewServeMux()
	public.HandleFunc("GET /healthz", h.health)
	public.HandleFunc("GET /api/version", h.version)
	public.HandleFunc("GET /api/need_token", h.needToken)

	protected := http.NewServeMux()
	protected.HandleFunc("GET /api/config", h.getConfig)
	protected.HandleFunc("POST /api/config", h.setConfig)
	protected.HandleFunc("GET /api/device/status", h.deviceStatus)
	protected.HandleFunc("GET /api/goform", h.goFormGet)
	protected.HandleFunc("POST /api/goform", h.goFormSet)
	protected.HandleFunc("GET /api/time/status", h.timeStatus)
	protected.HandleFunc("POST /api/time/sync", h.timeSync)
	protected.HandleFunc("POST /api/run_shell", h.runShell)
	protected.HandleFunc("GET /api/plugins/custom_head", h.getCustomHead)
	protected.HandleFunc("POST /api/plugins/custom_head", h.setCustomHead)
	protected.HandleFunc("GET /api/store/plugins", h.localCatalog)
	protected.HandleFunc("POST /api/store/plugins/{id}/install", h.installCatalogPlugin)
	protected.HandleFunc("GET /api/hosts", h.getHosts)
	protected.HandleFunc("POST /api/hosts", h.setHosts)
	protected.HandleFunc("POST /api/hosts/restore", h.restoreHosts)
	protected.HandleFunc("GET /api/scheduler", h.listSchedulers)
	protected.HandleFunc("POST /api/scheduler", h.setScheduler)
	protected.HandleFunc("DELETE /api/scheduler/{id}", h.deleteScheduler)
	protected.HandleFunc("GET /api/sms_forward_sms", h.getSMSForward)
	protected.HandleFunc("POST /api/sms_forward_sms", h.setSMSForward)
	protected.HandleFunc("GET /api/uploads", h.listUploads)
	protected.HandleFunc("POST /api/upload_file", h.uploadFile)
	protected.HandleFunc("GET /api/uploads/{name}", h.downloadUpload)
	protected.HandleFunc("GET /api/cellular/recent", h.recentCellular)
	protected.HandleFunc("POST /api/cellular/recent", h.recordCellular)
	h.registerCompatibilityRoutes(protected)

	root := http.NewServeMux()
	root.Handle("GET /healthz", public)
	root.Handle("GET /api/version", public)
	root.Handle("GET /api/need_token", public)
	assets, _ := fs.Sub(webui.Files, "static")
	root.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(assets))))
	root.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, webui.Files, "static/index.html")
	})
	root.Handle("/", h.auth.middleware(protected))
	return limitBody(securityHeaders(root))
}

func (h *handler) health(w http.ResponseWriter, r *http.Request) {
	if err := h.deps.Store.Ready(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, envelope{Code: 503, Message: "storage unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: map[string]bool{"ok": true}})
}

func (h *handler) version(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: map[string]string{
		"name": "UFI-TOOLS-U60-PRO-RE", "version": h.deps.Version,
	}})
}

func (h *handler) needToken(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"need_token": h.auth.required()})
}

func (h *handler) getConfig(w http.ResponseWriter, r *http.Request) {
	nickname, err := h.deps.Store.Get(r.Context(), "config", "nickname")
	if errors.Is(err, sql.ErrNoRows) {
		nickname = h.deps.Config.Nickname
	} else if err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "read configuration"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: map[string]any{
		"nickname":             nickname,
		"plugin_server_url":    h.deps.Config.PluginServerURL,
		"resource_server_url":  h.deps.Config.ResourceServerURL,
		"legacy_auth_enabled":  h.deps.Config.LegacyAuthEnabled,
		"legacy_shell_enabled": h.deps.Config.LegacyShellEnabled,
	}})
}

func (h *handler) setConfig(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Nickname string `json:"nickname"`
	}
	if err := decodeJSON(r, &request); err != nil || request.Nickname == "" {
		writeJSON(w, http.StatusBadRequest, envelope{Code: 400, Message: "nickname is required"})
		return
	}
	if err := h.deps.Store.Set(r.Context(), "config", "nickname", request.Nickname); err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "save configuration"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0})
}

func (h *handler) deviceStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.deps.Device.Status(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, envelope{Code: 502, Message: "device unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: status})
}

func (h *handler) goFormGet(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("cmd")
	if key == "" {
		key = r.URL.Query().Get("key")
	}
	value, err := h.deps.Device.GoFormGet(r.Context(), key)
	if err != nil {
		writeJSON(w, http.StatusNotFound, envelope{Code: 404, Message: "unsupported goform key"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: map[string]string{key: value}})
}

func (h *handler) goFormSet(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := decodeJSON(r, &request); err != nil || request.Key == "" {
		writeJSON(w, http.StatusBadRequest, envelope{Code: 400, Message: "key is required"})
		return
	}
	if err := h.deps.Device.GoFormSet(r.Context(), request.Key, request.Value); err != nil {
		writeJSON(w, http.StatusBadGateway, envelope{Code: 502, Message: "goform operation failed"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0})
}

func (h *handler) timeStatus(w http.ResponseWriter, _ *http.Request) {
	now := time.Now()
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: map[string]any{
		"unix": now.Unix(), "rfc3339": now.Format(time.RFC3339), "timezone": now.Location().String(),
	}})
}

func (h *handler) timeSync(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusNotImplemented, envelope{Code: 501, Message: "host time mutation is unavailable in simulator mode"})
}

func (h *handler) runShell(w http.ResponseWriter, _ *http.Request) {
	if !h.deps.Config.LegacyShellEnabled {
		writeJSON(w, http.StatusForbidden, envelope{Code: 403, Message: "legacy shell is disabled"})
		return
	}
	writeJSON(w, http.StatusNotImplemented, envelope{Code: 501, Message: "no command runner is configured"})
}

func (h *handler) getCustomHead(w http.ResponseWriter, r *http.Request) {
	source, err := h.deps.Store.Get(r.Context(), "kv", "custom_head")
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusOK, envelope{Code: 0, Data: map[string]any{"source": "", "plugins": []any{}}})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "read plugins"})
		return
	}
	doc, err := plugins.Parse(source)
	if err != nil {
		writeJSON(w, http.StatusOK, envelope{Code: 0, Data: map[string]any{"source": source, "valid": false}})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: doc})
}

func (h *handler) setCustomHead(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Source string `json:"source"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, envelope{Code: 400, Message: "invalid plugin document"})
		return
	}
	doc, err := plugins.Parse(request.Source)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, envelope{Code: 422, Message: err.Error()})
		return
	}
	if err := h.deps.Store.Set(r.Context(), "kv", "custom_head", request.Source); err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "save plugins"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: doc})
}

func (h *handler) localCatalog(w http.ResponseWriter, r *http.Request) {
	if h.deps.Catalog != nil {
		items, err := h.deps.Catalog.List(r.Context())
		if err != nil {
			writeJSON(w, http.StatusBadGateway, envelope{Code: 502, Message: "plugin catalog unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, envelope{Code: 0, Data: map[string]any{"source": "configured", "plugins": items}})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: map[string]any{
		"source": "local", "plugins": []any{}, "remote_configured": h.deps.Config.PluginServerURL != "",
	}})
}

func (h *handler) installCatalogPlugin(w http.ResponseWriter, r *http.Request) {
	if h.deps.Catalog == nil {
		writeJSON(w, http.StatusConflict, envelope{Code: 409, Message: "no plugin catalog configured"})
		return
	}
	item, data, err := h.deps.Catalog.Download(r.Context(), r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, envelope{Code: 502, Message: "plugin download or integrity check failed"})
		return
	}
	key := "plugin." + item.ID
	if err := h.deps.Store.Set(r.Context(), "kv", key, string(data)); err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "save plugin"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: item})
}

func (h *handler) getHosts(w http.ResponseWriter, r *http.Request) {
	value, err := h.deps.Store.Get(r.Context(), "kv", "hosts")
	if errors.Is(err, sql.ErrNoRows) {
		value = ""
	} else if err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "read hosts"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: map[string]string{"content": value}})
}

func (h *handler) setHosts(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Content string `json:"content"`
	}
	if err := decodeJSON(r, &request); err != nil || len(request.Content) > 256<<10 {
		writeJSON(w, http.StatusBadRequest, envelope{Code: 400, Message: "invalid hosts content"})
		return
	}
	if err := h.deps.Store.Set(r.Context(), "kv", "hosts", request.Content); err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "save hosts"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Message: "stored; device apply adapter is not configured"})
}

func (h *handler) restoreHosts(w http.ResponseWriter, r *http.Request) {
	if err := h.deps.Store.Set(r.Context(), "kv", "hosts", ""); err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "restore hosts"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0})
}

func (h *handler) listSchedulers(w http.ResponseWriter, r *http.Request) {
	items, err := h.deps.Store.ListSchedulers(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "list schedulers"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: items})
}

func (h *handler) setScheduler(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ID   string          `json:"id"`
		Data json.RawMessage `json:"data"`
	}
	if err := decodeJSON(r, &request); err != nil || request.ID == "" || len(request.Data) == 0 {
		writeJSON(w, http.StatusBadRequest, envelope{Code: 400, Message: "id and data are required"})
		return
	}
	if err := h.deps.Store.SetScheduler(r.Context(), request.ID, string(request.Data)); err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "save scheduler"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0})
}

func (h *handler) deleteScheduler(w http.ResponseWriter, r *http.Request) {
	if err := h.deps.Store.DeleteScheduler(r.Context(), r.PathValue("id")); err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "delete scheduler"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0})
}

func (h *handler) getSMSForward(w http.ResponseWriter, r *http.Request) {
	value, err := h.deps.Store.GetSMSForward(r.Context())
	if errors.Is(err, sql.ErrNoRows) {
		value = `{}`
	} else if err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "read SMS forwarding"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: json.RawMessage(value)})
}

func (h *handler) setSMSForward(w http.ResponseWriter, r *http.Request) {
	var value json.RawMessage
	if err := decodeJSON(r, &value); err != nil || len(value) == 0 {
		writeJSON(w, http.StatusBadRequest, envelope{Code: 400, Message: "invalid SMS forwarding configuration"})
		return
	}
	if err := h.deps.Store.SetSMSForward(r.Context(), string(value)); err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "save SMS forwarding"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Message: "stored; forwarding worker is not configured"})
}

func (h *handler) uploadFile(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, envelope{Code: 400, Message: "invalid multipart upload"})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, envelope{Code: 400, Message: "file is required"})
		return
	}
	defer file.Close()
	name := filepath.Base(header.Filename)
	if name == "." || name == "" || strings.ContainsRune(name, '\x00') {
		writeJSON(w, http.StatusBadRequest, envelope{Code: 400, Message: "invalid file name"})
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		writeJSON(w, http.StatusRequestEntityTooLarge, envelope{Code: 413, Message: "upload exceeds 1 MiB"})
		return
	}
	if err := h.deps.Store.PutUpload(r.Context(), name, data); err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "save upload"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: map[string]any{"name": name, "size": len(data)}})
}

func (h *handler) listUploads(w http.ResponseWriter, r *http.Request) {
	items, err := h.deps.Store.ListUploads(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "list uploads"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: items})
}

func (h *handler) downloadUpload(w http.ResponseWriter, r *http.Request) {
	data, err := h.deps.Store.GetUpload(r.Context(), r.PathValue("name"))
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "read upload"})
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="download"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *handler) recordCellular(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Timestamp int64 `json:"ts"`
		Total     int64 `json:"total"`
	}
	if err := decodeJSON(r, &request); err != nil || request.Timestamp <= 0 || request.Total < 0 {
		writeJSON(w, http.StatusBadRequest, envelope{Code: 400, Message: "invalid cellular point"})
		return
	}
	if err := h.deps.Store.RecordCellular(r.Context(), request.Timestamp, request.Total); err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "record cellular point"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0})
}

func (h *handler) recentCellular(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.deps.Store.RecentCellular(r.Context(), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, envelope{Code: 500, Message: "read cellular history"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: items})
}

func decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("multiple JSON values")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; object-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}
