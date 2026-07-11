// SPDX-License-Identifier: AGPL-3.0-or-later

package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/config"
)

type authenticator struct {
	cfg config.Config
	now func() time.Time
}

func (a authenticator) required() bool {
	return a.cfg.DeviceToken != "" || a.cfg.LegacyAuthEnabled
}

func (a authenticator) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.required() || a.tokenOK(r) || a.legacySignatureOK(r) {
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
	if provided == "" {
		provided = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	if provided == "" {
		provided = strings.TrimPrefix(r.Header.Get("kano-Authorization"), "Bearer ")
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
