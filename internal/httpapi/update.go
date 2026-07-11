// SPDX-License-Identifier: AGPL-3.0-or-later

package httpapi

import (
	"context"
	"net/http"
	"time"
)

func (h *handler) updateCheck(w http.ResponseWriter, r *http.Request) {
	if h.deps.Updater == nil {
		writeJSON(w, http.StatusConflict, envelope{Code: 409, Message: "no signed update source configured"})
		return
	}
	manifest, err := h.deps.Updater.Check(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, envelope{Code: 502, Message: "signed update check failed"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: manifest})
}

func (h *handler) updateDownload(w http.ResponseWriter, r *http.Request) {
	if h.deps.Updater == nil {
		writeJSON(w, http.StatusConflict, envelope{Code: 409, Message: "no signed update source configured"})
		return
	}
	status, err := h.deps.Updater.Download(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, envelope{Code: 502, Message: "signed update download failed", Data: status})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: status})
}

func (h *handler) updateStatus(w http.ResponseWriter, _ *http.Request) {
	if h.deps.Updater == nil {
		writeJSON(w, http.StatusOK, envelope{Code: 0, Data: map[string]string{"state": "unconfigured"}})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Code: 0, Data: h.deps.Updater.Status()})
}

func (h *handler) updateInstall(w http.ResponseWriter, _ *http.Request) {
	if h.deps.Updater == nil {
		writeJSON(w, http.StatusConflict, envelope{Code: 409, Message: "no signed update source configured"})
		return
	}
	if h.deps.Updater.Status().State != "ready" {
		writeJSON(w, http.StatusConflict, envelope{Code: 409, Message: "no verified update ready"})
		return
	}
	writeJSON(w, http.StatusAccepted, envelope{Code: 0, Message: "verified update installation started"})
	go func() {
		time.Sleep(250 * time.Millisecond)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		_ = h.deps.Updater.Install(ctx)
	}()
}
