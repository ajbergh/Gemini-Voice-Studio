// Copyright 2025 ajbergh
// SPDX-License-Identifier: Apache-2.0

// Package handler — api_voices.go implements voice discovery, casting,
// single/multi-speaker generation, formatting, and streaming TTS endpoints.
package handler

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/gemini"
	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/store"
)

// VoicesHandler handles /api/voices endpoints.
type VoicesHandler struct {
	Store         *store.Store
	KeysHandler   *KeysHandler
	AudioCacheDir string
	ProgressHub   *ProgressHub
}

// Recommend proxies AI casting requests to Gemini.
func (h *VoicesHandler) Recommend(w http.ResponseWriter, r *http.Request) {
	apiKey, err := h.KeysHandler.GetDecryptedKey("gemini")
	if err != nil {
		writeError(w, http.StatusPreconditionFailed, "no Gemini API key configured — add one via Settings")
		return
	}
	var request gemini.RecommendRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	request.Query = strings.TrimSpace(request.Query)
	if request.Query == "" {
		writeError(w, http.StatusBadRequest, "query is required")
		return
	}
	jobID := fmt.Sprintf("recommend_%d", time.Now().UnixMilli())
	h.emitVoiceProgress(jobID, "recommend", "processing", "Finding matching voices...", 10)

	voices := request.Voices
	if len(voices) == 0 {
		voices, err = h.getVoiceData()
		if err != nil {
			h.emitVoiceProgress(jobID, "recommend", "error", "Failed to load voice data", 0)
			writeError(w, http.StatusInternalServerError, "failed to load voice data")
			return
		}
	}
	result, err := gemini.NewClient(apiKey).Recommend(request.Query, voices)
	if err != nil {
		h.emitVoiceProgress(jobID, "recommend", "error", "AI casting failed", 0)
		slog.Error("gemini recommend failed", "error", err)
		writeError(w, http.StatusBadGateway, "AI recommendation failed")
		return
	}
	h.emitVoiceProgress(jobID, "recommend", "complete", "Voice matches ready", 100)

	resultJSON, _ := json.Marshal(result)
	if err := h.Store.InsertHistory(store.HistoryEntry{
		Type: "recommendation", InputText: request.Query, ResultJSON: strPtr(string(resultJSON)),
	}); err != nil {
		slog.Warn("failed to persist recommendation history", "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"voiceNames": result.RecommendedVoices, "systemInstruction": result.SystemInstruction,
		"sampleText": result.SampleText, "personDescription": result.PersonDescription,
	})
}

// GenerateTTS proxies a cancellable single-speaker TTS request.
func (h *VoicesHandler) GenerateTTS(w http.ResponseWriter, r *http.Request) {
	var request gemini.TTSRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(request.Text) == "" || strings.TrimSpace(request.VoiceName) == "" {
		writeError(w, http.StatusBadRequest, "text and voiceName are required")
		return
	}
	if err := gemini.ValidateTTSModel(request.Model); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	apiKey, err := h.KeysHandler.GetDecryptedKey("gemini")
	if err != nil {
		writeError(w, http.StatusPreconditionFailed, "no Gemini API key configured — add one via Settings")
		return
	}
	jobID := fmt.Sprintf("tts_%d", time.Now().UnixMilli())
	h.emitVoiceProgress(jobID, "tts", "processing", "Generating speech for "+request.VoiceName+"...", 10)

	audioBase64, err := gemini.NewClient(apiKey).GenerateTTSContext(
		r.Context(), request.Text, request.VoiceName, request.SystemInstruction, request.LanguageCode, request.Model,
	)
	if err != nil {
		h.emitVoiceProgress(jobID, "tts", "error", "TTS generation failed", 0)
		slog.Error("TTS failed", "error", err, "provider", request.Provider, "voice", request.VoiceName)
		writeError(w, http.StatusBadGateway, "TTS generation failed: "+err.Error())
		return
	}
	h.emitVoiceProgress(jobID, "tts", "complete", "Audio ready", 100)

	audioPath, persistErr := persistGeneratedAudio(h.AudioCacheDir, "tts", request.VoiceName, audioBase64)
	if persistErr != nil {
		slog.Warn("failed to persist generated TTS audio", "error", persistErr)
	}
	voiceName := request.VoiceName
	if err := h.Store.InsertHistory(store.HistoryEntry{
		Type: "tts", VoiceName: &voiceName, InputText: request.Text, AudioPath: audioPath,
	}); err != nil {
		slog.Warn("failed to persist TTS history", "error", err)
	}
	writeJSON(w, http.StatusOK, gemini.TTSResponse{AudioBase64: audioBase64})
}

// GenerateMultiSpeakerTTS proxies cancellable dialogue generation.
func (h *VoicesHandler) GenerateMultiSpeakerTTS(w http.ResponseWriter, r *http.Request) {
	var request gemini.MultiSpeakerTTSRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(request.Text) == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}
	if len(request.Speakers) < 1 || len(request.Speakers) > 2 {
		writeError(w, http.StatusBadRequest, "1 or 2 speakers are required")
		return
	}
	for _, speaker := range request.Speakers {
		if strings.TrimSpace(speaker.Speaker) == "" || strings.TrimSpace(speaker.VoiceName) == "" {
			writeError(w, http.StatusBadRequest, "each speaker must have a speaker name and voiceName")
			return
		}
	}
	if err := gemini.ValidateTTSModel(request.Model); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	apiKey, err := h.KeysHandler.GetDecryptedKey("gemini")
	if err != nil {
		writeError(w, http.StatusPreconditionFailed, "no Gemini API key configured — add one via Settings")
		return
	}
	jobID := fmt.Sprintf("multi_tts_%d", time.Now().UnixMilli())
	h.emitVoiceProgress(jobID, "multi_tts", "processing", "Generating dialogue audio...", 10)

	audioBase64, err := gemini.NewClient(apiKey).GenerateMultiSpeakerTTSContext(
		r.Context(), request.Text, request.Speakers, request.LanguageCode, request.Model,
	)
	if err != nil {
		h.emitVoiceProgress(jobID, "multi_tts", "error", "Dialogue generation failed", 0)
		slog.Error("gemini multi-speaker TTS failed", "error", err, "speaker_count", len(request.Speakers))
		writeError(w, http.StatusBadGateway, "Multi-speaker TTS generation failed: "+err.Error())
		return
	}
	h.emitVoiceProgress(jobID, "multi_tts", "complete", "Dialogue audio ready", 100)

	audioPath, persistErr := persistGeneratedAudio(h.AudioCacheDir, "tts_multi", "dialogue", audioBase64)
	if persistErr != nil {
		slog.Warn("failed to persist dialogue audio", "error", persistErr)
	}
	if err := h.Store.InsertHistory(store.HistoryEntry{Type: "tts_multi", InputText: request.Text, AudioPath: audioPath}); err != nil {
		slog.Warn("failed to persist dialogue history", "error", err)
	}
	writeJSON(w, http.StatusOK, gemini.TTSResponse{AudioBase64: audioBase64})
}

// ListVoices returns the voice library data from the backend catalogue.
func (h *VoicesHandler) ListVoices(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Store.DB().Query(
		"SELECT name, pitch, gender, characteristics, audio_sample_url, file_uri, analysis_json, image_url FROM voices ORDER BY name",
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query voices")
		return
	}
	defer rows.Close()
	voices := make([]map[string]any, 0)
	for rows.Next() {
		var name, pitch, gender, characteristicsJSON, audioURL, fileURI, analysisJSON, imageURL string
		if err := rows.Scan(&name, &pitch, &gender, &characteristicsJSON, &audioURL, &fileURI, &analysisJSON, &imageURL); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan voice")
			return
		}
		var characteristics []string
		_ = json.Unmarshal([]byte(characteristicsJSON), &characteristics)
		var analysis map[string]any
		_ = json.Unmarshal([]byte(analysisJSON), &analysis)
		voices = append(voices, map[string]any{
			"name": name, "pitch": pitch, "gender": gender, "characteristics": characteristics,
			"audioSampleUrl": audioURL, "fileUri": fileURI, "analysis": analysis, "imageUrl": imageURL,
		})
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read voice catalogue")
		return
	}
	writeJSON(w, http.StatusOK, voices)
}

func (h *VoicesHandler) getVoiceData() ([]gemini.VoiceData, error) {
	rows, err := h.Store.DB().Query("SELECT name, gender, pitch, characteristics FROM voices ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	voices := make([]gemini.VoiceData, 0)
	for rows.Next() {
		var voice gemini.VoiceData
		var characteristics string
		if err := rows.Scan(&voice.Name, &voice.Gender, &voice.Pitch, &characteristics); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(characteristics), &voice.Characteristics)
		voices = append(voices, voice)
	}
	return voices, rows.Err()
}

func strPtr(value string) *string { return &value }

// FormatScript sends script text to Gemini for TTS-optimized reformatting.
func (h *VoicesHandler) FormatScript(w http.ResponseWriter, r *http.Request) {
	apiKey, err := h.KeysHandler.GetDecryptedKey("gemini")
	if err != nil {
		writeError(w, http.StatusPreconditionFailed, "no Gemini API key configured — add one via Settings")
		return
	}
	var request struct {
		Script string `json:"script"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(request.Script) == "" {
		writeError(w, http.StatusBadRequest, "script is required")
		return
	}
	jobID := fmt.Sprintf("script_format_%d", time.Now().UnixMilli())
	h.emitVoiceProgress(jobID, "script_prep", "processing", "Formatting script...", 10)
	formatted, err := gemini.NewClient(apiKey).FormatScript(request.Script)
	if err != nil {
		h.emitVoiceProgress(jobID, "script_prep", "error", "Script formatting failed", 0)
		slog.Error("FormatScript failed", "error", err)
		writeError(w, http.StatusBadGateway, "failed to format script")
		return
	}
	h.emitVoiceProgress(jobID, "script_prep", "complete", "Script formatted", 100)
	writeJSON(w, http.StatusOK, map[string]string{"formatted": formatted})
}

// GenerateTTSStream relays provider SSE audio as application SSE events.
func (h *VoicesHandler) GenerateTTSStream(w http.ResponseWriter, r *http.Request) {
	var request gemini.TTSRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(request.Text) == "" || strings.TrimSpace(request.VoiceName) == "" {
		writeError(w, http.StatusBadRequest, "text and voiceName are required")
		return
	}
	if err := gemini.ValidateTTSModel(request.Model); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	effectiveModel := request.Model
	if effectiveModel == "" {
		effectiveModel = "gemini-3.1-flash-tts-preview"
	}
	if metadata, ok := gemini.GetTTSModel(effectiveModel); !ok || !metadata.Streaming {
		writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("TTS model %q does not support streaming", effectiveModel))
		return
	}
	apiKey, err := h.KeysHandler.GetDecryptedKey("gemini")
	if err != nil {
		writeError(w, http.StatusPreconditionFailed, "no Gemini API key configured")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	jobID := fmt.Sprintf("tts_stream_%d", time.Now().UnixMilli())
	h.emitVoiceProgress(jobID, "tts_stream", "processing", "Streaming speech for "+request.VoiceName+"...", 10)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	chunks := make(chan gemini.StreamTTSChunk, 16)
	errorChannel := make(chan error, 1)
	go func() {
		errorChannel <- gemini.NewClient(apiKey).GenerateTTSStreamContext(
			r.Context(), request.Text, request.VoiceName, request.SystemInstruction, request.LanguageCode, effectiveModel, chunks,
		)
	}()
	for chunk := range chunks {
		data, _ := json.Marshal(chunk)
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return
		}
		flusher.Flush()
	}
	if err := <-errorChannel; err != nil {
		h.emitVoiceProgress(jobID, "tts_stream", "error", "Streaming speech failed", 0)
		slog.Error("streaming TTS failed", "error", err)
		data, _ := json.Marshal(map[string]string{"error": err.Error()})
		_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", data)
		flusher.Flush()
		return
	}
	h.emitVoiceProgress(jobID, "tts_stream", "complete", "Streaming speech complete", 100)
}

func (h *VoicesHandler) emitVoiceProgress(jobID, jobType, status, message string, percent int) {
	if h.ProgressHub != nil {
		h.ProgressHub.EmitProgress(jobID, jobType, status, message, percent)
	}
}

func persistGeneratedAudio(directory, prefix, label, audioBase64 string) (*string, error) {
	if strings.TrimSpace(directory) == "" {
		return nil, nil
	}
	bytes, err := base64.StdEncoding.DecodeString(audioBase64)
	if err != nil {
		return nil, fmt.Errorf("decode generated audio: %w", err)
	}
	if len(bytes) == 0 {
		return nil, fmt.Errorf("generated audio is empty")
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create media directory: %w", err)
	}
	filename := fmt.Sprintf("%s_%d_%s.raw", sanitizeForFilename(prefix), time.Now().UnixNano(), sanitizeForFilename(label))
	finalPath, ok := safeCachePath(directory, filename)
	if !ok {
		return nil, fmt.Errorf("invalid media path")
	}
	temporary, err := os.CreateTemp(directory, ".audio-*.raw")
	if err != nil {
		return nil, err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return nil, err
	}
	if _, err := temporary.Write(bytes); err != nil {
		_ = temporary.Close()
		return nil, err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return nil, err
	}
	if err := temporary.Close(); err != nil {
		return nil, err
	}
	if err := os.Rename(temporaryPath, finalPath); err != nil {
		return nil, err
	}
	return &finalPath, nil
}
