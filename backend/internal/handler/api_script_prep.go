// Copyright 2025 ajbergh
// SPDX-License-Identifier: Apache-2.0

// Package handler — api_script_prep.go implements AI script preparation and
// reviewed-result application for project workflows.
package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/gemini"
	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/store"
)

// ScriptPrepHandler handles AI script preparation requests.
type ScriptPrepHandler struct {
	Store       *store.Store
	KeysHandler *KeysHandler
}

// PrepareScript analyzes and persists a structured narration plan.
func (h *ScriptPrepHandler) PrepareScript(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return
	}
	if _, err := h.Store.GetProject(projectID); err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	var request struct {
		RawScript string                   `json:"raw_script"`
		Options   gemini.ScriptPrepOptions `json:"options"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(request.RawScript) == "" {
		writeError(w, http.StatusBadRequest, "raw_script is required")
		return
	}
	apiKey, err := h.KeysHandler.GetDecryptedKey("gemini")
	if err != nil {
		slog.Error("script prep: no active API key", "error", err)
		writeError(w, http.StatusPreconditionFailed, "no active API key configured")
		return
	}
	job, err := h.Store.CreateScriptPrepJob(projectID, request.RawScript)
	if err != nil {
		slog.Error("script prep: create job", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create script prep job")
		return
	}
	if err := h.Store.UpdateScriptPrepJobResult(job.ID, "", "processing", ""); err != nil {
		slog.Warn("script prep: mark processing", "error", err)
	}

	result, providerErr := gemini.NewClient(apiKey).PrepareScriptForNarration(request.RawScript, request.Options)
	if providerErr != nil {
		slog.Error("script prep: Gemini error", "error", providerErr)
		_ = h.Store.UpdateScriptPrepJobResult(job.ID, "", "failed", providerErr.Error())
		writeError(w, http.StatusBadGateway, "Gemini script preparation failed")
		return
	}
	resultBytes, err := json.Marshal(result)
	if err != nil {
		_ = h.Store.UpdateScriptPrepJobResult(job.ID, "", "failed", "marshal error")
		writeError(w, http.StatusInternalServerError, "failed to serialize script prep result")
		return
	}
	if err := h.Store.UpdateScriptPrepJobResult(job.ID, string(resultBytes), "complete", ""); err != nil {
		slog.Error("script prep: persist result", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to persist script prep result")
		return
	}
	updated, err := h.Store.GetScriptPrepJob(job.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reload script prep result")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// GetLatestPrepResult returns null when no prep job exists, allowing the typed
// frontend client to distinguish an empty state from a transport failure.
func (h *ScriptPrepHandler) GetLatestPrepResult(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return
	}
	job, err := h.Store.GetLatestScriptPrepJob(projectID)
	if err != nil {
		slog.Error("script prep: get latest job", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve script prep result")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// ApplyScriptPrep applies a reviewed prep result atomically to a project.
func (h *ScriptPrepHandler) ApplyScriptPrep(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return
	}
	if _, err := h.Store.GetProject(projectID); err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	var request struct {
		JobID                      *int64                  `json:"job_id,omitempty"`
		Result                     *store.ScriptPrepResult `json:"result,omitempty"`
		CreateCastProfiles         bool                    `json:"create_cast_profiles"`
		CreatePronunciationEntries bool                    `json:"create_pronunciation_entries"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var result store.ScriptPrepResult
	switch {
	case request.Result != nil:
		result = *request.Result
	case request.JobID != nil:
		job, err := h.Store.GetScriptPrepJob(*request.JobID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to retrieve prep job")
			return
		}
		if job == nil || job.ProjectID != projectID {
			writeError(w, http.StatusNotFound, "prep job not found")
			return
		}
		if job.Status != "complete" || job.ResultJSON == nil || *job.ResultJSON == "" {
			writeError(w, http.StatusConflict, "prep job is not complete")
			return
		}
		if err := json.Unmarshal([]byte(*job.ResultJSON), &result); err != nil {
			writeError(w, http.StatusUnprocessableEntity, "prep job result is invalid")
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "job_id or result is required")
		return
	}

	summary, err := h.Store.ApplyScriptPrepResult(projectID, result, store.ScriptPrepApplyOptions{
		CreateCastProfiles: request.CreateCastProfiles,
		CreatePronunciationEntries: request.CreatePronunciationEntries,
	})
	if err != nil {
		slog.Error("script prep: apply result", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to apply prep result")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}
