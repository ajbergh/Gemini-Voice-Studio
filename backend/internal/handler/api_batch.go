// Copyright 2025 ajbergh
// SPDX-License-Identifier: Apache-2.0

// Package handler — api_batch.go implements bounded-concurrency, cancellable
// project rendering and single-segment production renders.
package handler

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	audioanalysis "github.com/ajbergh/gemini-voice-gen-tts/backend/internal/audio"
	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/gemini"
	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/promptbuilder"
	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/pronunciation"
	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/store"
)

const maxBatchConcurrency = 8

// BatchHandler handles batch render and job cancellation endpoints.
type BatchHandler struct {
	Store         *store.Store
	KeysHandler   *KeysHandler
	AudioCacheDir string
	ProgressHub   *ProgressHub

	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

type batchRenderBody struct {
	SegmentIDs       []int64 `json:"segment_ids,omitempty"`
	SegmentIDsLegacy []int64 `json:"segmentIds,omitempty"`
	Force            bool    `json:"force,omitempty"`
	Concurrency      int     `json:"concurrency,omitempty"`
}

type batchRenderResponse struct {
	JobID        string `json:"job_id"`
	SegmentCount int    `json:"segment_count"`
	Concurrency  int    `json:"concurrency"`
}

type renderTask struct {
	segment        store.ScriptSegment
	originalStatus string
}

type renderResult struct {
	task renderTask
	err  error
}

// BatchRenderProject enqueues eligible project segments and returns immediately.
func (h *BatchHandler) BatchRenderProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parsePathInt64(w, r, "id", "invalid project ID")
	if !ok {
		return
	}
	var body batchRenderBody
	if r.ContentLength > 0 {
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}
	project, err := h.Store.GetProject(projectID)
	if err != nil {
		writeStoreError(w, err, "project not found", "failed to get project")
		return
	}
	allSegments, err := h.Store.ListProjectSegments(projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list segments")
		return
	}
	segmentIDs := body.SegmentIDs
	if len(segmentIDs) == 0 {
		segmentIDs = body.SegmentIDsLegacy
	}
	segments := filterRenderableSegments(allSegments, segmentIDs, body.Force)
	if len(segments) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "no renderable segments found")
		return
	}

	concurrency := h.resolveBatchConcurrency(body.Concurrency)
	if concurrency > len(segments) {
		concurrency = len(segments)
	}
	jobID := fmt.Sprintf("batch_%d_%d", projectID, time.Now().UnixMilli())
	ctx, cancel := context.WithCancel(context.Background())
	h.mu.Lock()
	if h.cancels == nil {
		h.cancels = make(map[string]context.CancelFunc)
	}
	h.cancels[jobID] = cancel
	h.mu.Unlock()

	if h.ProgressHub != nil {
		h.ProgressHub.Broadcast(ProgressEvent{
			JobID: jobID, Type: "batch_render", Status: "queued",
			Message: fmt.Sprintf("Queued %d segments with %d worker(s)", len(segments), concurrency),
			TotalItems: len(segments), ProjectID: fmt.Sprintf("%d", projectID),
		})
	}
	go h.runBatchRender(ctx, jobID, projectID, project, segments, concurrency)
	writeJSON(w, http.StatusAccepted, batchRenderResponse{JobID: jobID, SegmentCount: len(segments), Concurrency: concurrency})
}

// RenderSegment renders one segment synchronously and returns its newly-created take.
func (h *BatchHandler) RenderSegment(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parsePathInt64(w, r, "id", "invalid project ID")
	if !ok {
		return
	}
	segmentID, ok := parsePathInt64(w, r, "segmentId", "invalid segment ID")
	if !ok {
		return
	}
	project, err := h.Store.GetProject(projectID)
	if err != nil {
		writeStoreError(w, err, "project not found", "failed to get project")
		return
	}
	segments, err := h.Store.ListProjectSegments(projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list segments")
		return
	}
	var segment *store.ScriptSegment
	for index := range segments {
		if segments[index].ID == segmentID {
			segment = &segments[index]
			break
		}
	}
	if segment == nil {
		writeError(w, http.StatusNotFound, "segment not found")
		return
	}
	if strings.TrimSpace(segment.ScriptText) == "" {
		writeError(w, http.StatusUnprocessableEntity, "segment has no script text")
		return
	}

	originalStatus := segment.Status
	_ = h.Store.UpdateSegmentStatus(projectID, segmentID, "rendering")
	if err := h.renderOneSegment(r.Context(), projectID, project, *segment); err != nil {
		status := "failed"
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			status = originalStatus
		}
		_ = h.Store.UpdateSegmentStatus(projectID, segmentID, status)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	takes, err := h.Store.ListSegmentTakes(projectID, segmentID)
	if err != nil || len(takes) == 0 {
		writeError(w, http.StatusInternalServerError, "render completed without a persisted take")
		return
	}
	writeJSON(w, http.StatusOK, takes[0])
}

// CancelJob cancels a running batch render job. The operation is idempotent.
func (h *BatchHandler) CancelJob(w http.ResponseWriter, r *http.Request) {
	jobID := strings.TrimSpace(r.PathValue("id"))
	if jobID == "" {
		writeError(w, http.StatusBadRequest, "job ID is required")
		return
	}
	h.mu.Lock()
	cancel, found := h.cancels[jobID]
	if found {
		delete(h.cancels, jobID)
	}
	h.mu.Unlock()
	if !found {
		writeJSON(w, http.StatusOK, map[string]string{"status": "not_found"})
		return
	}
	cancel()
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (h *BatchHandler) runBatchRender(ctx context.Context, jobID string, projectID int64, project *store.ScriptProject, segments []store.ScriptSegment, concurrency int) {
	defer func() {
		h.mu.Lock()
		delete(h.cancels, jobID)
		h.mu.Unlock()
	}()

	total := len(segments)
	projectIDText := fmt.Sprintf("%d", projectID)
	emit := func(status, message string, processed, completed, failed int, segmentID int64) {
		if h.ProgressHub == nil {
			return
		}
		event := ProgressEvent{
			JobID: jobID, Type: "batch_render", Status: status, Message: message,
			Percent: completedPercent(processed, total), TotalItems: total,
			CompletedItems: completed, FailedItems: failed, ProjectID: projectIDText,
		}
		if segmentID > 0 {
			event.SegmentID = fmt.Sprintf("%d", segmentID)
		}
		h.ProgressHub.Broadcast(event)
	}
	emit("running", fmt.Sprintf("Rendering %d segments with %d worker(s)", total, concurrency), 0, 0, 0, 0)

	tasks := make(chan renderTask)
	results := make(chan renderResult, concurrency)
	var workers sync.WaitGroup
	for workerID := 0; workerID < concurrency; workerID++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for task := range tasks {
				if ctx.Err() != nil {
					return
				}
				_ = h.Store.UpdateSegmentStatus(projectID, task.segment.ID, "rendering")
				err := h.renderOneSegment(ctx, projectID, project, task.segment)
				results <- renderResult{task: task, err: err}
			}
		}()
	}
	go func() {
		defer close(tasks)
		for _, segment := range segments {
			task := renderTask{segment: segment, originalStatus: segment.Status}
			select {
			case tasks <- task:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		workers.Wait()
		close(results)
	}()

	processed, completed, failed := 0, 0, 0
	for result := range results {
		processed++
		if result.err != nil {
			if errors.Is(result.err, context.Canceled) || errors.Is(result.err, context.DeadlineExceeded) {
				_ = h.Store.UpdateSegmentStatus(projectID, result.task.segment.ID, result.task.originalStatus)
			} else {
				failed++
				_ = h.Store.UpdateSegmentStatus(projectID, result.task.segment.ID, "failed")
				slog.Error("batch render: segment failed", "segment_id", result.task.segment.ID, "error", result.err)
			}
		} else {
			completed++
		}
		emit("running", fmt.Sprintf("Processed %d of %d segments", processed, total), processed, completed, failed, result.task.segment.ID)
	}

	if ctx.Err() != nil {
		emit("cancelled", fmt.Sprintf("Cancelled after %d/%d segments", processed, total), processed, completed, failed, 0)
		return
	}
	status := "complete"
	if failed == total {
		status = "failed"
	} else if failed > 0 {
		status = "partial"
	}
	emit(status, fmt.Sprintf("Rendered %d/%d segments (%d failed)", completed, total, failed), total, completed, failed, 0)
}

func (h *BatchHandler) resolveBatchConcurrency(requested int) int {
	value := requested
	if value <= 0 {
		value = 2
		if raw := strings.TrimSpace(h.Store.GetConfigValue(store.ConfigKeyDefaultBatchConcurrency, "2")); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil {
				value = parsed
			}
		}
	}
	if value < 1 {
		value = 1
	}
	if value > maxBatchConcurrency {
		value = maxBatchConcurrency
	}
	return value
}

// renderOneSegment resolves production settings, generates audio, durably
// persists the file and take, creates automated QC, and only then marks the
// segment rendered.
func (h *BatchHandler) renderOneSegment(ctx context.Context, projectID int64, project *store.ScriptProject, segment store.ScriptSegment) error {
	voiceName := derefStr(segment.VoiceName)
	castLanguage := ""
	var promptInput promptbuilder.Input
	var castProfileID *int64
	var presetID *int64
	if segment.CastProfileID != nil {
		if profile, err := h.Store.GetCastProfile(*segment.CastProfileID); err == nil {
			castProfileID = segment.CastProfileID
			if profile.VoiceName != nil && *profile.VoiceName != "" {
				voiceName = *profile.VoiceName
			}
			if profile.LanguageCode != nil {
				castLanguage = *profile.LanguageCode
			}
			promptInput.CastRole = profile.Role
			promptInput.CastDescription = profile.Description
			promptInput.CastPronunciationNotes = derefStr(profile.PronunciationNotes)
			if segment.StyleID == nil && profile.StyleID != nil {
				segment.StyleID = profile.StyleID
			}
			if segment.PresetID == nil && profile.PresetID != nil {
				segment.PresetID = profile.PresetID
			}
		}
	}
	presetID = segment.PresetID
	if presetID == nil {
		presetID = project.DefaultPresetID
	}
	if presetID != nil {
		if preset, err := h.Store.GetCustomPreset(*presetID); err == nil {
			if voiceName == "" {
				voiceName = preset.VoiceName
			}
			promptInput.PresetInstruction = derefStr(preset.SystemInstruction)
		}
	}
	if voiceName == "" {
		voiceName = derefStr(project.DefaultVoiceName)
	}
	if voiceName == "" {
		return fmt.Errorf("segment %d: no voice configured", segment.ID)
	}

	provider := h.resolveProvider(project, segment)
	model := h.resolveModel(provider, project, segment)
	if err := gemini.ValidateTTSModel(model); err != nil {
		return fmt.Errorf("segment %d: %w", segment.ID, err)
	}
	languageCode := derefStr(segment.LanguageCode)
	if languageCode == "" {
		languageCode = castLanguage
	}
	if languageCode == "" {
		languageCode = derefStr(project.DefaultLanguageCode)
	}
	if languageCode == "" {
		languageCode, _ = h.Store.GetConfig(store.ConfigKeyDefaultLanguageCode)
	}

	appVoiceName := voiceName
	providerVoice, err := h.resolveProviderVoice(projectID, "gemini", appVoiceName, provider)
	if err != nil {
		return fmt.Errorf("segment %d: %w", segment.ID, err)
	}
	fallbackProvider := h.resolveFallbackProvider(project, segment)
	fallbackModel := h.resolveFallbackModel(fallbackProvider, project, segment)
	fallbackVoice := ""
	if fallbackProvider != "" && fallbackProvider != provider {
		fallbackVoice, err = h.resolveProviderVoice(projectID, "gemini", appVoiceName, fallbackProvider)
		if err != nil {
			return fmt.Errorf("segment %d fallback: %w", segment.ID, err)
		}
	}

	styleID := segment.StyleID
	if styleID == nil {
		styleID = project.DefaultStyleID
	}
	if styleID != nil {
		if style, err := h.Store.GetStyle(*styleID); err == nil {
			promptInput.StyleName = style.Name
			promptInput.StyleDirectorNotes = style.DirectorNotes
			promptInput.StylePacing = derefStr(style.Pacing)
			promptInput.StyleEnergy = derefStr(style.Energy)
			promptInput.StyleEmotion = derefStr(style.Emotion)
			promptInput.StyleArticulation = derefStr(style.Articulation)
			promptInput.StylePauseDensity = derefStr(style.PauseDensity)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	renderText := segment.ScriptText
	dictionaryHash := ""
	if entries, err := h.Store.ListEnabledEntriesForProject(projectID); err == nil && len(entries) > 0 {
		dictionaryHash = hashPronunciationEntries(entries)
		renderText = pronunciation.ApplyDictionary(renderText, entries)
	}
	systemInstruction, promptHash := promptbuilder.Compose(promptInput)
	audioBase64, usedProvider, usedModel, usedVoice, usedFallback, err := h.generateWithFallback(
		ctx, segment, renderText, systemInstruction, languageCode,
		provider, model, providerVoice, fallbackProvider, fallbackModel, fallbackVoice,
	)
	if err != nil {
		return fmt.Errorf("segment %d: TTS: %w", segment.ID, err)
	}
	audioBytes, err := base64.StdEncoding.DecodeString(audioBase64)
	if err != nil {
		return fmt.Errorf("segment %d: decode audio: %w", segment.ID, err)
	}
	if len(audioBytes) == 0 {
		return fmt.Errorf("segment %d: provider returned empty audio", segment.ID)
	}
	if strings.TrimSpace(h.AudioCacheDir) == "" {
		return fmt.Errorf("segment %d: audio storage is not configured", segment.ID)
	}
	if err := os.MkdirAll(h.AudioCacheDir, 0o700); err != nil {
		return fmt.Errorf("segment %d: create audio storage: %w", segment.ID, err)
	}

	safeVoice := sanitizeForFilename(usedVoice)
	filename := fmt.Sprintf("take_%d_%d_%d_%s.raw", projectID, segment.ID, time.Now().UnixNano(), safeVoice)
	finalPath, ok := safeCachePath(h.AudioCacheDir, filename)
	if !ok {
		return fmt.Errorf("segment %d: invalid audio path", segment.ID)
	}
	temporary, err := os.CreateTemp(h.AudioCacheDir, ".render-*.raw")
	if err != nil {
		return fmt.Errorf("segment %d: create temporary audio: %w", segment.ID, err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("segment %d: secure temporary audio: %w", segment.ID, err)
	}
	if _, err := temporary.Write(audioBytes); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("segment %d: write temporary audio: %w", segment.ID, err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("segment %d: sync temporary audio: %w", segment.ID, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("segment %d: close temporary audio: %w", segment.ID, err)
	}
	if err := os.Rename(temporaryPath, finalPath); err != nil {
		return fmt.Errorf("segment %d: publish audio: %w", segment.ID, err)
	}
	published := true
	defer func() {
		if published {
			_ = os.Remove(finalPath)
		}
	}()

	metrics := audioanalysis.AnalyzePCM16LE(audioBytes, audioanalysis.DefaultSampleRate, audioanalysis.DefaultChannels)
	sampleRate, channels, format := metrics.SampleRate, metrics.Channels, metrics.Format
	providerForTake, providerVoiceForTake, voiceForTake := usedProvider, usedVoice, appVoiceName
	var languageForTake, modelForTake, instructionForTake *string
	if languageCode != "" {
		languageForTake = &languageCode
	}
	if usedModel != "" {
		modelForTake = &usedModel
	}
	if systemInstruction != "" {
		instructionForTake = &systemInstruction
	}
	settingsJSON := marshalRenderSettings(map[string]any{
		"requested_provider": provider, "requested_model": model, "provider": usedProvider,
		"model": usedModel, "provider_voice": usedVoice, "app_voice_name": appVoiceName,
		"fallback_provider": fallbackProvider, "fallback_model": fallbackModel,
		"used_fallback": usedFallback, "dictionary_hash": dictionaryHash, "prompt_hash": promptHash,
	})
	takeID, err := h.Store.CreateTake(store.SegmentTake{
		ProjectID: projectID, SegmentID: segment.ID, VoiceName: &voiceForTake,
		SpeakerLabel: segment.SpeakerLabel, LanguageCode: languageForTake,
		Provider: &providerForTake, Model: modelForTake, ProviderVoice: &providerVoiceForTake,
		AppVoiceName: &voiceForTake, PresetID: presetID, StyleID: styleID,
		AccentID: segment.AccentID, CastProfileID: castProfileID,
		DictionaryHash: optionalStringPtr(dictionaryHash), PromptHash: optionalStringPtr(promptHash),
		SettingsJSON: settingsJSON, SystemInstruction: instructionForTake, ScriptText: segment.ScriptText,
		AudioPath: &finalPath, DurationSeconds: &metrics.DurationSeconds,
		PeakDbfs: finiteFloatPtr(metrics.PeakDbfs), RmsDbfs: finiteFloatPtr(metrics.RmsDbfs),
		ClippingDetected: metrics.ClippingDetected, SampleRate: &sampleRate, Channels: &channels,
		Format: &format, Status: "rendered",
	})
	if err != nil {
		return fmt.Errorf("segment %d: persist take: %w", segment.ID, err)
	}
	published = false

	if h.shouldCreateClippingIssue(metrics) {
		note := fmt.Sprintf("Rendered audio peaks at %.2f dBFS and should be reviewed for limiter artifacts.", metrics.PeakDbfs)
		if metrics.ClippingDetected {
			note = "Rendered audio contains clipped PCM samples and should be reviewed for distortion."
		}
		if _, err := h.Store.CreateQcIssue(store.QcIssue{
			ProjectID: projectID, SegmentID: segment.ID, TakeID: &takeID,
			IssueType: "volume", Severity: "high", Note: note, Status: "open",
		}); err != nil {
			slog.Warn("batch render: create clipping QC issue", "segment_id", segment.ID, "take_id", takeID, "error", err)
		}
	}
	if err := h.Store.UpdateSegmentStatus(projectID, segment.ID, "rendered"); err != nil {
		// The take and audio are already durable. Do not report the provider render
		// as failed or delete the asset; surface the metadata repair need in logs.
		slog.Error("render persisted but segment status update failed", "segment_id", segment.ID, "take_id", takeID, "error", err)
	}
	return nil
}

func (h *BatchHandler) shouldCreateClippingIssue(metrics audioanalysis.Analysis) bool {
	if !strings.EqualFold(h.Store.GetConfigValue(store.ConfigKeyQcAutoFlagClipping, "true"), "true") {
		return false
	}
	if metrics.ClippingDetected {
		return true
	}
	if math.IsInf(metrics.PeakDbfs, 0) || math.IsNaN(metrics.PeakDbfs) {
		return false
	}
	threshold := -0.1
	if raw := strings.TrimSpace(h.Store.GetConfigValue(store.ConfigKeyQcClippingThresholdDb, "-0.1")); raw != "" {
		if parsed, err := strconv.ParseFloat(raw, 64); err == nil {
			threshold = parsed
		}
	}
	return metrics.PeakDbfs >= threshold
}

func filterRenderableSegments(segments []store.ScriptSegment, ids []int64, force bool) []store.ScriptSegment {
	selected := make(map[int64]bool, len(ids))
	for _, id := range ids {
		selected[id] = true
	}
	out := make([]store.ScriptSegment, 0, len(segments))
	for _, segment := range segments {
		if len(selected) > 0 && !selected[segment.ID] {
			continue
		}
		if !force && segment.Status != "draft" && segment.Status != "changed" && segment.Status != "failed" {
			continue
		}
		if strings.TrimSpace(segment.ScriptText) == "" {
			continue
		}
		out = append(out, segment)
	}
	return out
}

func derefStr(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func finiteFloatPtr(value float64) *float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nil
	}
	return &value
}

func optionalStringPtr(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func marshalRenderSettings(value map[string]any) *string {
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	out := string(data)
	return &out
}

func hashPronunciationEntries(entries []store.PronunciationEntry) string {
	if len(entries) == 0 {
		return ""
	}
	hash := sha256.New()
	for _, entry := range entries {
		_, _ = fmt.Fprintf(hash, "%d|%d|%s|%s|%t|%t|%d\n",
			entry.ID, entry.DictionaryID, entry.RawWord, entry.Replacement, entry.IsRegex, entry.Enabled, entry.SortOrder)
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func (h *BatchHandler) resolveProvider(project *store.ScriptProject, segment store.ScriptSegment) string {
	if value := derefStr(segment.Provider); value != "" {
		return normalizeProvider(value)
	}
	if value := derefStr(project.DefaultProvider); value != "" {
		return normalizeProvider(value)
	}
	if project.ClientID != nil {
		if client, err := h.Store.GetClient(*project.ClientID); err == nil {
			if value := derefStr(client.DefaultProvider); value != "" {
				return normalizeProvider(value)
			}
		}
	}
	if value, _ := h.Store.GetConfig(store.ConfigKeyDefaultProvider); strings.TrimSpace(value) != "" {
		return normalizeProvider(value)
	}
	return "gemini"
}

func (h *BatchHandler) resolveModel(provider string, project *store.ScriptProject, segment store.ScriptSegment) string {
	if value := derefStr(segment.Model); value != "" {
		return value
	}
	if normalizeProviderIfSet(derefStr(segment.Provider)) != "" {
		return defaultModelForProvider(provider)
	}
	if value := derefStr(project.DefaultModel); value != "" && defaultModelUsableForProvider(provider, derefStr(project.DefaultProvider), value) {
		return value
	}
	if project.ClientID != nil {
		if client, err := h.Store.GetClient(*project.ClientID); err == nil {
			if value := derefStr(client.DefaultModel); value != "" && defaultModelUsableForProvider(provider, derefStr(client.DefaultProvider), value) {
				return value
			}
		}
	}
	if value, _ := h.Store.GetConfig(store.ConfigKeyDefaultModel); strings.TrimSpace(value) != "" {
		globalProvider, _ := h.Store.GetConfig(store.ConfigKeyDefaultProvider)
		if defaultModelUsableForProvider(provider, globalProvider, value) {
			return strings.TrimSpace(value)
		}
	}
	return defaultModelForProvider(provider)
}

func (h *BatchHandler) resolveFallbackProvider(project *store.ScriptProject, segment store.ScriptSegment) string {
	if value := derefStr(segment.FallbackProvider); value != "" {
		return normalizeProvider(value)
	}
	if value := derefStr(project.FallbackProvider); value != "" {
		return normalizeProvider(value)
	}
	if project.ClientID != nil {
		if client, err := h.Store.GetClient(*project.ClientID); err == nil {
			if value := derefStr(client.FallbackProvider); value != "" {
				return normalizeProvider(value)
			}
		}
	}
	if value, _ := h.Store.GetConfig(store.ConfigKeyFallbackProvider); strings.TrimSpace(value) != "" {
		return normalizeProvider(value)
	}
	return ""
}

func (h *BatchHandler) resolveFallbackModel(provider string, project *store.ScriptProject, segment store.ScriptSegment) string {
	if provider == "" {
		return ""
	}
	if value := derefStr(segment.FallbackModel); value != "" {
		return value
	}
	if normalizeProviderIfSet(derefStr(segment.FallbackProvider)) != "" {
		return defaultModelForProvider(provider)
	}
	if value := derefStr(project.FallbackModel); value != "" && defaultModelUsableForProvider(provider, derefStr(project.FallbackProvider), value) {
		return value
	}
	if project.ClientID != nil {
		if client, err := h.Store.GetClient(*project.ClientID); err == nil {
			if value := derefStr(client.FallbackModel); value != "" && defaultModelUsableForProvider(provider, derefStr(client.FallbackProvider), value) {
				return value
			}
		}
	}
	if value, _ := h.Store.GetConfig(store.ConfigKeyFallbackModel); strings.TrimSpace(value) != "" {
		globalProvider, _ := h.Store.GetConfig(store.ConfigKeyFallbackProvider)
		if defaultModelUsableForProvider(provider, globalProvider, value) {
			return strings.TrimSpace(value)
		}
	}
	return defaultModelForProvider(provider)
}

func (h *BatchHandler) resolveProviderVoice(projectID int64, sourceProvider, sourceVoice, targetProvider string) (string, error) {
	_ = projectID
	_ = sourceProvider
	_ = targetProvider
	return sourceVoice, nil
}

func (h *BatchHandler) generateWithFallback(ctx context.Context, segment store.ScriptSegment, text, instruction, languageCode, provider, model, voice, fallbackProvider, fallbackModel, fallbackVoice string) (audioBase64, usedProvider, usedModel, usedVoice string, usedFallback bool, err error) {
	audioBase64, err = h.generateProviderTTS(ctx, provider, model, voice, text, instruction, languageCode)
	if err == nil {
		return audioBase64, provider, model, voice, false, nil
	}
	if fallbackProvider == "" || fallbackProvider == provider || !fallbackAllowedForSegment(segment) {
		return "", provider, model, voice, false, err
	}
	if fallbackModel == "" {
		fallbackModel = defaultModelForProvider(fallbackProvider)
	}
	audioBase64, fallbackErr := h.generateProviderTTS(ctx, fallbackProvider, fallbackModel, fallbackVoice, text, instruction, languageCode)
	if fallbackErr != nil {
		return "", provider, model, voice, false, fmt.Errorf("%w; fallback %s also failed: %v", err, fallbackProvider, fallbackErr)
	}
	return audioBase64, fallbackProvider, fallbackModel, fallbackVoice, true, nil
}

func (h *BatchHandler) generateProviderTTS(ctx context.Context, provider, model, voice, text, instruction, languageCode string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if normalizeProvider(provider) != "gemini" {
		return "", fmt.Errorf("unsupported provider %q", provider)
	}
	if err := gemini.ValidateTTSModel(model); err != nil {
		return "", err
	}
	apiKey, err := h.KeysHandler.GetDecryptedKey("gemini")
	if err != nil {
		return "", fmt.Errorf("no Gemini API key: %w", err)
	}
	return gemini.NewClient(apiKey).GenerateTTSContext(ctx, text, voice, instruction, languageCode, model)
}

func fallbackAllowedForSegment(segment store.ScriptSegment) bool {
	switch strings.ToLower(segment.Status) {
	case "approved", "locked":
		return false
	default:
		return true
	}
}

func defaultModelForProvider(provider string) string {
	provider = normalizeProvider(provider)
	for _, registered := range registry {
		if strings.EqualFold(registered.ID, provider) {
			return registered.DefaultModel
		}
	}
	return "gemini-3.1-flash-tts-preview"
}

func defaultModelUsableForProvider(provider, configuredProvider, model string) bool {
	if configuredProvider = normalizeProviderIfSet(configuredProvider); configuredProvider != "" && configuredProvider != provider {
		return false
	}
	return modelCompatibleWithProvider(provider, model)
}

func modelCompatibleWithProvider(provider, model string) bool {
	model = strings.TrimSpace(model)
	if model == "" {
		return true
	}
	provider = normalizeProvider(provider)
	for _, registered := range registry {
		if !strings.EqualFold(registered.ID, provider) {
			continue
		}
		for _, candidate := range registered.Models {
			if strings.EqualFold(candidate.ID, model) {
				return true
			}
		}
		return false
	}
	return false
}

func normalizeProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", "google", "google-gemini", "openai":
		return "gemini"
	default:
		return strings.ToLower(strings.TrimSpace(provider))
	}
}

func normalizeProviderIfSet(provider string) string {
	if strings.TrimSpace(provider) == "" {
		return ""
	}
	return normalizeProvider(provider)
}

func completedPercent(done, total int) int {
	if total <= 0 {
		return 0
	}
	percentage := (done * 100) / total
	if percentage > 100 {
		return 100
	}
	return percentage
}
