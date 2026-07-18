// Copyright 2025 ajbergh
// SPDX-License-Identifier: Apache-2.0

// Package handler implements shared HTTP helpers and endpoint handlers.
package handler

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/buildinfo"
)

// Health returns stable liveness and release metadata without exposing secrets.
func Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":     "ok",
		"version":    buildinfo.Version,
		"commit":     buildinfo.Commit,
		"build_date": buildinfo.Date,
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":       errorCodeForStatus(status),
			"message":    message,
			"retryable":  status == http.StatusTooManyRequests || status >= 500,
			"http_status": status,
		},
	})
}

func errorCodeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "INVALID_REQUEST"
	case http.StatusUnauthorized:
		return "UNAUTHORIZED"
	case http.StatusForbidden:
		return "FORBIDDEN"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusConflict:
		return "CONFLICT"
	case http.StatusPreconditionFailed:
		return "PRECONDITION_FAILED"
	case http.StatusUnprocessableEntity:
		return "UNPROCESSABLE_ENTITY"
	case http.StatusTooManyRequests:
		return "RATE_LIMITED"
	case http.StatusBadGateway:
		return "UPSTREAM_FAILURE"
	default:
		if status >= 500 {
			return "INTERNAL_ERROR"
		}
		return "REQUEST_FAILED"
	}
}

// decodeJSON reads and decodes one JSON value, rejects trailing content, and
// limits bodies to 10 MiB.
func decodeJSON(r *http.Request, value any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 10<<20)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return &json.SyntaxError{}
	}
	return nil
}

var safeFilenameRe = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

func sanitizeForFilename(value string) string {
	value = filepath.Base(value)
	value = strings.ReplaceAll(value, "..", "")
	value = safeFilenameRe.ReplaceAllString(value, "_")
	if value == "" || value == "." {
		value = "unknown"
	}
	return value
}

func safeCachePath(cacheDir, filename string) (string, bool) {
	path := filepath.Join(cacheDir, filename)
	clean := filepath.Clean(path)
	if !strings.HasPrefix(clean, filepath.Clean(cacheDir)+string(filepath.Separator)) {
		return "", false
	}
	return clean, true
}
