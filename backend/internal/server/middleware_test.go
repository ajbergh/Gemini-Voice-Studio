// Copyright 2025 ajbergh
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOriginProtectionMiddleware(t *testing.T) {
	handler := originProtectionMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	t.Run("allows trusted localhost origin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/keys", nil)
		req.Header.Set("Origin", "http://localhost:5173")
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		if res.Code != http.StatusNoContent {
			t.Fatalf("expected trusted origin to pass, got %d", res.Code)
		}
	})

	t.Run("rejects cross-site origin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "http://127.0.0.1/api/history", nil)
		req.Header.Set("Origin", "https://evil.example")
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		if res.Code != http.StatusForbidden {
			t.Fatalf("expected forbidden response, got %d", res.Code)
		}
	})

	t.Run("allows requests without browser origin headers", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/backup", nil)
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		if res.Code != http.StatusNoContent {
			t.Fatalf("expected non-browser request to pass, got %d", res.Code)
		}
	})
}

func TestLoggingMiddlewarePreservesFlusher(t *testing.T) {
	called := false
	handler := loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("logging middleware removed http.Flusher")
		}
		called = true
		_, _ = w.Write([]byte("data: ready\n\n"))
		flusher.Flush()
	}))

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/voices/tts/stream", nil))
	if !called {
		t.Fatal("wrapped handler was not called")
	}
	if !res.Flushed {
		t.Fatal("underlying response was not flushed")
	}
}

type hijackableRecorder struct {
	*httptest.ResponseRecorder
}

func (h *hijackableRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	server, client := net.Pipe()
	_ = client.Close()
	return server, bufio.NewReadWriter(bufio.NewReader(server), bufio.NewWriter(server)), nil
}

func TestLoggingMiddlewarePreservesHijacker(t *testing.T) {
	handler := loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("logging middleware removed http.Hijacker")
		}
		conn, _, err := hijacker.Hijack()
		if err != nil {
			t.Fatalf("hijack failed: %v", err)
		}
		_ = conn.Close()
	}))

	handler.ServeHTTP(&hijackableRecorder{ResponseRecorder: httptest.NewRecorder()}, httptest.NewRequest(http.MethodGet, "/api/ws/progress", nil))
}
