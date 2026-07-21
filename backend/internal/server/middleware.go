// Copyright 2025 ajbergh
// SPDX-License-Identifier: Apache-2.0

// Package server — middleware.go provides HTTP middleware for structured
// request logging (slog), panic recovery, CORS headers, origin protection, and
// an interface-preserving status-capturing ResponseWriter wrapper.
package server

import (
	"bufio"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// loggingMiddleware logs HTTP request method, path, status, and duration.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		slog.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration", time.Since(start).String(),
		)
	})
}

// recoveryMiddleware catches panics and returns 500.
func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("panic recovered", "error", err, "path", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// corsMiddleware adds CORS headers for the localhost development frontend.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && isLocalhostOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// originProtectionMiddleware rejects unsafe browser requests unless their
// Origin/Referer is localhost or exactly matches the request host. Exact
// same-origin matching supports the embedded app behind Docker or a trusted
// reverse proxy without allowing arbitrary cross-site writes.
func originProtectionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requiresTrustedOrigin(r.Method) || requestHasTrustedOrigin(r) {
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden origin"}`))
	})
}

// requiresTrustedOrigin returns true for methods that can mutate local state.
func requiresTrustedOrigin(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// requestHasTrustedOrigin validates Origin or Referer when the browser sends one.
func requestHasTrustedOrigin(r *http.Request) bool {
	if origin := r.Header.Get("Origin"); origin != "" {
		return isTrustedRequestOrigin(origin, r.Host)
	}
	if referer := r.Header.Get("Referer"); referer != "" {
		return isTrustedRequestOrigin(referer, r.Host)
	}
	return true
}

func isTrustedRequestOrigin(rawOrigin, requestHost string) bool {
	if isLocalhostOrigin(rawOrigin) {
		return true
	}
	parsed, err := url.Parse(rawOrigin)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return false
	}
	return requestHost != "" && strings.EqualFold(parsed.Host, requestHost)
}

// isLocalhostOrigin checks if an origin is localhost or a loopback IP.
func isLocalhostOrigin(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}

	switch host := parsed.Hostname(); {
	case strings.EqualFold(host, "localhost"):
		return true
	case host == "127.0.0.1":
		return true
	case host == "::1":
		return true
	default:
		return false
	}
}

// securityHeadersMiddleware adds standard security response headers.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// statusWriter wraps ResponseWriter to capture the status code while preserving
// optional transport interfaces required by SSE, WebSockets, and optimized file
// transfers. Unwrap also lets net/http.ResponseController reach the original
// writer through any future middleware layers.
type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

// Unwrap returns the underlying response writer.
func (sw *statusWriter) Unwrap() http.ResponseWriter { return sw.ResponseWriter }

// WriteHeader captures the first status code before delegating to the wrapped writer.
func (sw *statusWriter) WriteHeader(code int) {
	if sw.wroteHeader {
		return
	}
	sw.status = code
	sw.wroteHeader = true
	sw.ResponseWriter.WriteHeader(code)
}

// Write captures the implicit 200 response before delegating.
func (sw *statusWriter) Write(p []byte) (int, error) {
	if !sw.wroteHeader {
		sw.WriteHeader(http.StatusOK)
	}
	return sw.ResponseWriter.Write(p)
}

// Flush preserves http.Flusher for Server-Sent Events and streaming responses.
func (sw *statusWriter) Flush() {
	_ = http.NewResponseController(sw.ResponseWriter).Flush()
}

// Hijack preserves http.Hijacker for WebSocket upgrades under HTTP/1.1.
func (sw *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := sw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hj.Hijack()
}

// ReadFrom preserves the optimized io.ReaderFrom path used by file downloads.
func (sw *statusWriter) ReadFrom(r io.Reader) (int64, error) {
	if !sw.wroteHeader {
		sw.WriteHeader(http.StatusOK)
	}
	if rf, ok := sw.ResponseWriter.(io.ReaderFrom); ok {
		return rf.ReadFrom(r)
	}
	return io.Copy(sw.ResponseWriter, r)
}
