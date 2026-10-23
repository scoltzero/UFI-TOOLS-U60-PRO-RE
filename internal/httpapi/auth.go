// SPDX-License-Identifier: AGPL-3.0-or-later

package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/config"
	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/store"
)

type authenticator struct {
	cfg   config.Config
	store *store.Store
	now   func() time.Time
}

// Stored rotations take effect immediately and remain authoritative at the
// next process start. The configured token is the initial fallback only.
func (a authenticator) current(ctx context.Context) (authenticator, error) {
	if a.store == nil {
		return a, nil
	}
	token, err := a.store.Get(ctx, "config", "device_token")
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return a, err
	}
	if err == nil {
		if token == "" {
			return a, errors.New("stored application token is empty")
		}
		a.cfg.DeviceToken = token
	}
	a.store = nil
	return a, nil
}

func (a authenticator) required() bool {
	return a.cfg.DeviceToken != "" || a.cfg.LegacyAuthEnabled
}

func (a authenticator) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current, err := a.current(r.Context())
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, envelope{Code: 503, Message: "authentication storage unavailable"})
			return
		}
		if !current.required() || current.tokenOK(r) || current.legacySignatureOK(r) {
			next.ServeHTTP(w, r)
			return
		}
		writeJSON(w, http.StatusUnauthorized, envelope{Code: 401, Message: "authentication required"})
	})
}

func (a authenticator) tokenOK(r *http.Request) bool {
	if a.cfg.DeviceToken == "" {
		return false
	}
	provided := r.Header.Get("X-Device-Token")
	if strings.HasPrefix(r.URL.Path, "/api/terminal/") && r.URL.Path != "/api/terminal/session" {
		if cookie, err := r.Cookie("ufi_terminal"); err == nil && provided == "" {
			provided = cookie.Value
		}
	}
	if provided == "" {
		provided = r.Header.Get("Authorization")
		provided = strings.TrimPrefix(provided, "Bearer ")
	}
	if provided == "" {
		provided = r.Header.Get("kano-Authorization")
		provided = strings.TrimPrefix(provided, "Bearer ")
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(a.cfg.DeviceToken)) == 1
}

// legacySignatureOK implements the documented compatibility profile. It is
// deliberately opt-in because timestamped shared-secret signatures are weaker
// than the primary token mode and do not provide request-body integrity.
func (a authenticator) legacySignatureOK(r *http.Request) bool {
	if !a.cfg.LegacyAuthEnabled || a.cfg.DeviceToken == "" {
		return false
	}
	timestamp := r.Header.Get("kano-t")
	signature := r.Header.Get("kano-sign")
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || signature == "" {
		return false
	}
	now := a.now
	if now == nil {
		now = time.Now
	}
	delta := now().Sub(time.Unix(seconds, 0))
	if delta < -a.cfg.SignClockSkew || delta > a.cfg.SignClockSkew {
		return false
	}
	mac := hmac.New(sha256.New, []byte(a.cfg.DeviceToken))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("\n"))
	mac.Write([]byte(r.Method))
	mac.Write([]byte("\n"))
	mac.Write([]byte(r.URL.RequestURI()))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(strings.ToLower(signature)), []byte(expected))
}
