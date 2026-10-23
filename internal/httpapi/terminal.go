// SPDX-License-Identifier: AGPL-3.0-or-later
package httpapi

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
)

func (h *handler) terminalSession(w http.ResponseWriter, r *http.Request) {
	if !h.deps.Config.PrivilegedCompat {
		writeLegacyError(w, 403, "terminal control requires privileged mode")
		return
	}
	auth, err := h.auth.current(r.Context())
	if err != nil {
		writeLegacyError(w, 503, "authentication unavailable")
		return
	}
	if auth.cfg.DeviceToken == "" {
		writeLegacyError(w, 409, "terminal requires an application token")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "ufi_terminal", Value: auth.cfg.DeviceToken, Path: "/api/terminal/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil, MaxAge: 600})
	writeJSON(w, 200, envelope{Code: 0, Data: map[string]string{"url": "/api/terminal/"}})
}
func (h *handler) terminalProxy(w http.ResponseWriter, r *http.Request) {
	if !h.deps.Config.PrivilegedCompat || h.deps.TTYD == nil || !h.deps.TTYD.Status().Running {
		writeLegacyError(w, 409, "candidate terminal is stopped")
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || !strings.EqualFold(u.Host, r.Host) {
			writeLegacyError(w, 403, "terminal origin rejected")
			return
		}
	}
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	w.Header().Set("Content-Security-Policy", "default-src 'self' 'unsafe-inline' 'unsafe-eval' data: blob:; connect-src 'self' ws: wss:; frame-ancestors 'self'")
	target, _ := url.Parse("http://127.0.0.1:" + strconv.Itoa(h.deps.Config.TTYDPort))
	proxy := httputil.NewSingleHostReverseProxy(target)
	director := proxy.Director
	proxy.Director = func(req *http.Request) {
		director(req)
		req.Header.Del("Authorization")
		req.Header.Del("X-Device-Token")
		req.Header.Del("kano-Authorization")
		req.Header.Del("Cookie")
	}
	proxy.ServeHTTP(w, r)
}
