// SPDX-License-Identifier: AGPL-3.0-or-later

package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"
)

const maxProxyResponse = 32 << 20

var rootAsset = regexp.MustCompile(`(?i)(src|href)\s*=\s*(["'])/([^"']*)["']`)

var unsafeProxyHeaders = map[string]bool{
	"connection": true, "keep-alive": true, "proxy-authenticate": true,
	"proxy-authorization": true, "te": true, "trailer": true,
	"transfer-encoding": true, "upgrade": true, "host": true,
	"content-length": true, "expect": true, "authorization": true,
	"cookie": true, "set-cookie": true,
}

func (h *handler) compatProxy(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimPrefix(r.PathValue("path"), "/")
	if raw == "" {
		raw = strings.TrimPrefix(r.URL.Path, "/api/proxy/")
	}
	if strings.HasPrefix(raw, "--") {
		h.externalProxy(w, r, strings.TrimPrefix(raw, "--"))
		return
	}
	h.privateDeviceProxy(w, r, raw)
}

func (h *handler) privateDeviceProxy(w http.ResponseWriter, r *http.Request, raw string) {
	if h.deps.Config.DeviceBackend != "zte-http" || h.deps.Config.DeviceBaseURL == "" {
		writeLegacyError(w, http.StatusConflict, "ZTE HTTP backend is not configured")
		return
	}
	requested := "/" + strings.TrimPrefix(raw, "/")
	clean := path.Clean(requested)
	if clean != requested || !(strings.HasPrefix(clean, "/goform/") || strings.HasPrefix(clean, "/api/")) {
		writeLegacyError(w, http.StatusForbidden, "proxy path is not allowed")
		return
	}
	base, _ := url.Parse(h.deps.Config.DeviceBaseURL)
	base.Path, base.RawQuery = clean, r.URL.RawQuery
	h.forwardProxy(w, r, base, true)
}

func (h *handler) externalProxy(w http.ResponseWriter, r *http.Request, raw string) {
	target, err := url.Parse(raw)
	if err != nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") || target.User != nil {
		writeLegacyError(w, http.StatusBadRequest, "invalid proxy target")
		return
	}
	if err := validatePublicProxyTarget(r.Context(), target); err != nil {
		writeLegacyError(w, http.StatusForbidden, err.Error())
		return
	}
	h.forwardProxy(w, r, target, false)
}

func (h *handler) forwardProxy(w http.ResponseWriter, r *http.Request, target *url.URL, private bool) {
	var body io.Reader
	if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
		data, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
		if err != nil || len(data) > 1<<20 {
			writeLegacyError(w, http.StatusRequestEntityTooLarge, "proxy request body is too large")
			return
		}
		body = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), body)
	if err != nil {
		writeLegacyError(w, http.StatusBadRequest, "invalid proxy request")
		return
	}
	copyProxyRequestHeaders(request.Header, r.Header)
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("too many redirects")
			}
			if private {
				if next.URL.Host != target.Host {
					return errors.New("device proxy redirect changed origin")
				}
				return nil
			}
			return validatePublicProxyTarget(next.Context(), next.URL)
		},
	}
	if !private {
		client.Transport = &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           safePublicDial,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          16,
			IdleConnTimeout:       30 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 15 * time.Second,
		}
	}
	response, err := client.Do(request)
	if err != nil {
		writeLegacyError(w, http.StatusBadGateway, "proxy request failed")
		return
	}
	defer response.Body.Close()
	copyProxyResponseHeaders(w.Header(), response.Header)
	contentType := response.Header.Get("Content-Type")
	if strings.Contains(strings.ToLower(contentType), "text/html") {
		data, err := io.ReadAll(io.LimitReader(response.Body, maxProxyResponse+1))
		if err != nil || len(data) > maxProxyResponse {
			writeLegacyError(w, http.StatusBadGateway, "proxy HTML response is too large")
			return
		}
		if !private {
			prefix := "/api/proxy/--" + strings.TrimSuffix(target.Scheme+"://"+target.Host+path.Dir(target.Path), "/")
			data = rootAsset.ReplaceAll(data, []byte(`${1}=${2}`+prefix+`/${3}${2}`))
		}
		w.WriteHeader(response.StatusCode)
		_, _ = w.Write(data)
		return
	}
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, io.LimitReader(response.Body, maxProxyResponse))
}

func copyProxyRequestHeaders(target, source http.Header) {
	for name, values := range source {
		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, "kano-") {
			actual := strings.TrimSpace(name[len("kano-"):])
			if actual != "" && !unsafeProxyHeaders[strings.ToLower(actual)] {
				for _, value := range values {
					target.Add(actual, value)
				}
			} else if strings.EqualFold(actual, "cookie") || strings.EqualFold(actual, "authorization") {
				for _, value := range values {
					target.Add(actual, value)
				}
			}
			continue
		}
		if unsafeProxyHeaders[lower] || strings.HasPrefix(lower, "sec-") || strings.HasPrefix(lower, "x-forwarded-") {
			continue
		}
		for _, value := range values {
			target.Add(name, value)
		}
	}
	target.Set("Accept-Encoding", "identity")
}

func copyProxyResponseHeaders(target, source http.Header) {
	for name, values := range source {
		lower := strings.ToLower(name)
		if lower == "set-cookie" {
			for _, value := range values {
				target.Add("Kano-SetCk", value)
				target.Add("Kano-Set-Cookie", value)
			}
			continue
		}
		if unsafeProxyHeaders[lower] || strings.HasPrefix(lower, "content-security-policy") || lower == "clear-site-data" {
			continue
		}
		for _, value := range values {
			target.Add(name, value)
		}
	}
	target.Set("Access-Control-Expose-Headers", "Kano-SetCk, Kano-Set-Cookie")
}

func validatePublicProxyTarget(ctx context.Context, target *url.URL) error {
	host := target.Hostname()
	if host == "" || strings.EqualFold(host, "ufi.ztedevice.com") {
		return errors.New("proxy target is forbidden")
	}
	if ip := net.ParseIP(host); ip != nil {
		if unsafeProxyIP(ip) {
			return errors.New("proxy target resolved to a private or reserved address")
		}
		return nil
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(ips) == 0 {
		return errors.New("proxy target DNS lookup failed")
	}
	for _, address := range ips {
		if unsafeProxyIP(address.IP) {
			return errors.New("proxy target resolved to a private or reserved address")
		}
	}
	return nil
}

func unsafeProxyIP(ip net.IP) bool {
	return ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast()
}

func safePublicDial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(addresses) == 0 {
		return nil, errors.New("proxy target DNS lookup failed")
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	var lastErr error
	for _, candidate := range addresses {
		if unsafeProxyIP(candidate.IP) {
			return nil, errors.New("proxy target resolved to a private or reserved address")
		}
		connection, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
		if dialErr == nil {
			return connection, nil
		}
		lastErr = dialErr
	}
	return nil, lastErr
}
