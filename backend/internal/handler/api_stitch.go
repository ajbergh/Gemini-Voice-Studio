// Copyright 2025 ajbergh
// SPDX-License-Identifier: Apache-2.0

// Package handler — api_stitch.go implements project audio stitching with the
// same deterministic finishing primitives used by packaged exports.
package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	audiofinish "github.com/ajbergh/gemini-voice-gen-tts/backend/internal/audio"
	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/store"
)

// StitchHandler handles POST /api/projects/{id}/stitch.
type StitchHandler struct {
	Store         *store.Store
	AudioCacheDir string
}

type stitchRequest struct {
	ExportProfileID *int64 `json:"export_profile_id,omitempty"`
	SectionID       *int64 `json:"section_id,omitempty"`
}

// StitchProject concatenates approved/rendered takes into a single WAV file.
func (h *StitchHandler) StitchProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parsePathInt64(w, r, "id", "invalid project ID")
	if !ok {
		return
	}

	var req stitchRequest
	if r.ContentLength > 0 {
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}

	var profile *store.ExportProfile
	if req.ExportProfileID != nil {
		loaded, err := h.Store.GetExportProfile(*req.ExportProfileID)
		if err != nil {
			writeError(w, http.StatusNotFound, "export profile not found")
			return
		}
		profile = loaded
	}

	segments, err := h.Store.ListProjectSegments(projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list segments")
		return
	}
	if req.SectionID != nil {
		filtered := make([]store.ScriptSegment, 0, len(segments))
		for _, segment := range segments {
			if segment.SectionID != nil && *segment.SectionID == *req.SectionID {
				filtered = append(filtered, segment)
			}
		}
		segments = filtered
	}

	buffers := make([][]byte, 0, len(segments))
	for _, segment := range segments {
		take, err := h.Store.GetBestTakeForSegment(projectID, segment.ID)
		if err != nil || take == nil || take.AudioPath == nil {
			slog.Debug("stitch: segment skipped", "segment_id", segment.ID, "reason", "no audio")
			continue
		}
		pcm, err := readCachedAudioFile(h.AudioCacheDir, *take.AudioPath)
		if err != nil {
			slog.Warn("stitch: segment skipped", "segment_id", segment.ID, "error", err)
			continue
		}
		if profile != nil && profile.TrimSilence {
			pcm = audiofinish.TrimPCM16Silence(pcm, profile.SilenceThresholdDb)
		}
		buffers = append(buffers, pcm)
	}
	if len(buffers) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "no renderable segments with audio found")
		return
	}

	interSegmentMS := 500
	if profile != nil {
		interSegmentMS = profile.InterSegmentSilenceMs
	}
	separator := audiofinish.PCM16Silence(interSegmentMS, audiofinish.DefaultSampleRate)
	var allPCM []byte
	for index, buffer := range buffers {
		allPCM = append(allPCM, buffer...)
		if index < len(buffers)-1 {
			allPCM = append(allPCM, separator...)
		}
	}
	if profile != nil {
		allPCM = audiofinish.FinishPCM16(allPCM, audiofinish.FinishOptions{
			LeadingSilenceMS: profile.LeadingSilenceMs,
			TrailingSilenceMS: profile.TrailingSilenceMs,
			NormalizePeakDB: profile.NormalizePeakDb,
			SampleRate: audiofinish.DefaultSampleRate,
		})
	}

	wavData := audiofinish.EncodePCM16WAV(allPCM, audiofinish.DefaultSampleRate, audiofinish.DefaultChannels)
	filename := fmt.Sprintf("project-%d-export.wav", projectID)
	if req.SectionID != nil {
		filename = fmt.Sprintf("project-%d-section-%d-export.wav", projectID, *req.SectionID)
	}
	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(wavData)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(wavData)
}
