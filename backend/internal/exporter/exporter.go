// Copyright 2025 ajbergh
// SPDX-License-Identifier: Apache-2.0

// Package exporter builds bounded-memory ZIP deliverable archives from the
// selected project takes and applies the same finishing profile used by the
// stitched-WAV workflow.
package exporter

import (
	"archive/zip"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	audiofinish "github.com/ajbergh/gemini-voice-gen-tts/backend/internal/audio"
	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/store"
)

// Config holds the dependencies for an export run.
type Config struct {
	Store          *store.Store
	AudioCacheDir  string
	ExportCacheDir string
}

// Run executes the export job and updates its persisted state.
func Run(ctx context.Context, cfg Config, jobID, projectID int64) {
	if err := cfg.Store.UpdateExportJobStatus(jobID, "running", nil, nil); err != nil {
		slog.Error("export: mark running", "job_id", jobID, "error", err)
	}
	outputPath, err := run(ctx, cfg, jobID, projectID)
	if err != nil {
		message := err.Error()
		_ = cfg.Store.UpdateExportJobStatus(jobID, "failed", nil, &message)
		slog.Error("export: job failed", "job_id", jobID, "error", err)
		return
	}
	_ = cfg.Store.UpdateExportJobStatus(jobID, "complete", &outputPath, nil)
	slog.Info("export: job complete", "job_id", jobID, "output", outputPath)
}

func run(ctx context.Context, cfg Config, jobID, projectID int64) (outputPath string, returnErr error) {
	if err := os.MkdirAll(cfg.ExportCacheDir, 0o700); err != nil {
		return "", fmt.Errorf("create export cache dir: %w", err)
	}
	project, err := cfg.Store.GetProject(projectID)
	if err != nil || project == nil {
		return "", fmt.Errorf("load project %d: %w", projectID, err)
	}
	sections, err := cfg.Store.ListProjectSections(projectID)
	if err != nil {
		return "", fmt.Errorf("load sections: %w", err)
	}
	segments, err := cfg.Store.ListProjectSegments(projectID)
	if err != nil {
		return "", fmt.Errorf("load segments: %w", err)
	}

	var profile *store.ExportProfile
	job, err := cfg.Store.GetExportJob(jobID)
	if err != nil {
		return "", fmt.Errorf("load export job: %w", err)
	}
	if job != nil && job.ExportProfileID != nil {
		profile, err = cfg.Store.GetExportProfile(*job.ExportProfileID)
		if err != nil {
			return "", fmt.Errorf("load export profile: %w", err)
		}
	}

	outputPath = filepath.Join(cfg.ExportCacheDir, fmt.Sprintf("%d.zip", jobID))
	temporary, err := os.CreateTemp(cfg.ExportCacheDir, fmt.Sprintf(".%d-*.zip", jobID))
	if err != nil {
		return "", fmt.Errorf("create temporary export: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		if returnErr != nil {
			_ = os.Remove(temporaryPath)
		}
	}()

	zw := zip.NewWriter(temporary)
	defer func() {
		if returnErr != nil {
			_ = zw.Close()
		}
	}()

	requireApproved := strings.EqualFold(cfg.Store.GetConfigValue(store.ConfigKeyQcExportOnlyApproved, "false"), "true")
	type takeRef struct {
		TakeID       int64    `json:"take_id"`
		TakeNumber   int      `json:"take_number"`
		VoiceName    string   `json:"voice_name"`
		Provider     string   `json:"provider"`
		Model        string   `json:"model"`
		LanguageCode string   `json:"language_code"`
		DurationSec  *float64 `json:"duration_seconds,omitempty"`
		Status       string   `json:"status"`
		AudioFile    string   `json:"audio_file,omitempty"`
	}
	type segmentEntry struct {
		ID           int64    `json:"id"`
		SectionID    *int64   `json:"section_id,omitempty"`
		Title        string   `json:"title"`
		ScriptText   string   `json:"script_text"`
		SpeakerLabel string   `json:"speaker_label,omitempty"`
		Status       string   `json:"status"`
		SortOrder    int      `json:"sort_order"`
		BestTake     *takeRef `json:"best_take,omitempty"`
	}
	type sectionEntry struct {
		ID        int64  `json:"id"`
		Title     string `json:"title"`
		Kind      string `json:"kind"`
		SortOrder int    `json:"sort_order"`
	}

	sectionEntries := make([]sectionEntry, 0, len(sections))
	for _, section := range sections {
		sectionEntries = append(sectionEntries, sectionEntry{ID: section.ID, Title: section.Title, Kind: section.Kind, SortOrder: section.SortOrder})
	}

	segmentEntries := make([]segmentEntry, 0, len(segments))
	renderItems := make([]map[string]any, 0, len(segments))
	masterSegments := make([][]byte, 0, len(segments))
	audioIndex := 1
	for _, segment := range segments {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		entry := segmentEntry{
			ID: segment.ID, SectionID: segment.SectionID, Title: segment.Title,
			ScriptText: segment.ScriptText, SpeakerLabel: derefStr(segment.SpeakerLabel),
			Status: segment.Status, SortOrder: segment.SortOrder,
		}

		take, takeErr := cfg.Store.GetBestTakeForSegment(projectID, segment.ID)
		if takeErr != nil {
			return "", fmt.Errorf("load take for segment %d: %w", segment.ID, takeErr)
		}
		if take == nil || (requireApproved && take.Status != "approved") {
			segmentEntries = append(segmentEntries, entry)
			continue
		}
		if take.AudioPath == nil || strings.TrimSpace(*take.AudioPath) == "" {
			return "", fmt.Errorf("segment %d take %d has no audio path", segment.ID, take.ID)
		}
		pcmPath := filepath.Join(cfg.AudioCacheDir, filepath.Base(*take.AudioPath))
		pcm, err := os.ReadFile(pcmPath)
		if err != nil {
			return "", fmt.Errorf("read audio for segment %d: %w", segment.ID, err)
		}

		masterPCM := append([]byte(nil), pcm...)
		segmentPCM := append([]byte(nil), pcm...)
		if profile != nil {
			if profile.TrimSilence {
				masterPCM = append([]byte(nil), audiofinish.TrimPCM16Silence(masterPCM, profile.SilenceThresholdDb)...)
			}
			segmentPCM = audiofinish.FinishPCM16(segmentPCM, audiofinish.FinishOptions{
				TrimSilence: profile.TrimSilence, SilenceThresholdDB: profile.SilenceThresholdDb,
				LeadingSilenceMS: profile.LeadingSilenceMs, TrailingSilenceMS: profile.TrailingSilenceMs,
				NormalizePeakDB: profile.NormalizePeakDb, SampleRate: audiofinish.DefaultSampleRate,
			})
		}
		masterSegments = append(masterSegments, masterPCM)

		voice := derefStr(take.VoiceName)
		audioFile := fmt.Sprintf("audio/%03d-%s.wav", audioIndex, sanitizeFilename(voice))
		if err := writeBytes(zw, audioFile, audiofinish.EncodePCM16WAV(segmentPCM, audiofinish.DefaultSampleRate, audiofinish.DefaultChannels)); err != nil {
			return "", err
		}
		audioIndex++

		entry.BestTake = &takeRef{
			TakeID: take.ID, TakeNumber: take.TakeNumber, VoiceName: voice,
			Provider: derefStr(take.Provider), Model: derefStr(take.Model),
			LanguageCode: derefStr(take.LanguageCode), DurationSec: take.DurationSeconds,
			Status: take.Status, AudioFile: audioFile,
		}
		renderItems = append(renderItems, map[string]any{
			"segment_id": segment.ID, "take_id": take.ID, "take_number": take.TakeNumber,
			"voice_name": voice, "provider": derefStr(take.Provider), "model": derefStr(take.Model),
			"language_code": derefStr(take.LanguageCode), "app_voice_name": derefStr(take.AppVoiceName),
			"provider_voice": derefStr(take.ProviderVoice), "prompt_hash": derefStr(take.PromptHash),
			"dictionary_hash": derefStr(take.DictionaryHash), "status": take.Status, "audio_file": audioFile,
		})
		segmentEntries = append(segmentEntries, entry)
	}

	if len(masterSegments) > 0 {
		interSegmentMS := 500
		if profile != nil {
			interSegmentMS = profile.InterSegmentSilenceMs
		}
		separator := audiofinish.PCM16Silence(interSegmentMS, audiofinish.DefaultSampleRate)
		var masterPCM []byte
		for index, segmentPCM := range masterSegments {
			masterPCM = append(masterPCM, segmentPCM...)
			if index < len(masterSegments)-1 {
				masterPCM = append(masterPCM, separator...)
			}
		}
		if profile != nil {
			masterPCM = audiofinish.FinishPCM16(masterPCM, audiofinish.FinishOptions{
				LeadingSilenceMS: profile.LeadingSilenceMs, TrailingSilenceMS: profile.TrailingSilenceMs,
				NormalizePeakDB: profile.NormalizePeakDb, SampleRate: audiofinish.DefaultSampleRate,
			})
		}
		if err := writeBytes(zw, "audio/project-master.wav", audiofinish.EncodePCM16WAV(masterPCM, audiofinish.DefaultSampleRate, audiofinish.DefaultChannels)); err != nil {
			return "", err
		}
	}

	projectDocument := map[string]any{
		"exported_at": time.Now().UTC().Format(time.RFC3339),
		"project": project, "sections": sectionEntries, "segments": segmentEntries,
	}
	if err := writeJSON(zw, "project.json", projectDocument); err != nil {
		return "", err
	}
	castProfiles, err := cfg.Store.ListCastProfiles(projectID)
	if err != nil {
		return "", fmt.Errorf("load cast profiles: %w", err)
	}
	if err := writeJSON(zw, "cast-bible.json", map[string]any{"cast_profiles": castProfiles}); err != nil {
		return "", err
	}
	pronunciations, err := cfg.Store.ListEnabledEntriesForProject(projectID)
	if err != nil {
		return "", fmt.Errorf("load pronunciation entries: %w", err)
	}
	if err := writeJSON(zw, "pronunciation-dictionary.json", map[string]any{"entries": pronunciations}); err != nil {
		return "", err
	}
	qcIssues, err := cfg.Store.ListProjectQcIssues(projectID, "")
	if err != nil {
		return "", fmt.Errorf("load QC issues: %w", err)
	}
	if err := writeQcCSV(zw, qcIssues); err != nil {
		return "", err
	}
	if err := writeJSON(zw, "render-metadata.json", map[string]any{
		"exported_at": time.Now().UTC().Format(time.RFC3339),
		"profile": profile, "takes": renderItems,
		"audio_format": map[string]any{"sample_rate": audiofinish.DefaultSampleRate, "channels": audiofinish.DefaultChannels, "bit_depth": 16},
	}); err != nil {
		return "", err
	}
	if err := writeBytes(zw, "README.txt", []byte(buildReadme(project.Title, audioIndex-1, profile))); err != nil {
		return "", err
	}

	if err := zw.Close(); err != nil {
		return "", fmt.Errorf("close zip: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return "", fmt.Errorf("sync zip: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("close zip file: %w", err)
	}
	_ = os.Remove(outputPath)
	if err := os.Rename(temporaryPath, outputPath); err != nil {
		return "", fmt.Errorf("publish zip: %w", err)
	}
	return outputPath, nil
}

var unsafeChars = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

func sanitizeFilename(name string) string {
	value := strings.ToLower(strings.TrimSpace(name))
	value = unsafeChars.ReplaceAllString(value, "_")
	if len(value) > 32 {
		value = value[:32]
	}
	if value == "" {
		value = "voice"
	}
	return value
}

func derefStr(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func writeJSON(zw *zip.Writer, name string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", name, err)
	}
	return writeBytes(zw, name, data)
}

func writeBytes(zw *zip.Writer, name string, data []byte) error {
	writer, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("create zip entry %s: %w", name, err)
	}
	if _, err := writer.Write(data); err != nil {
		return fmt.Errorf("write zip entry %s: %w", name, err)
	}
	return nil
}

func writeQcCSV(zw *zip.Writer, issues []store.QcIssue) error {
	writer, err := zw.Create("qc-notes.csv")
	if err != nil {
		return fmt.Errorf("create qc-notes.csv: %w", err)
	}
	csvWriter := csv.NewWriter(writer)
	_ = csvWriter.Write([]string{"id", "segment_id", "take_id", "issue_type", "severity", "status", "note", "created_at"})
	for _, issue := range issues {
		takeID := ""
		if issue.TakeID != nil {
			takeID = fmt.Sprintf("%d", *issue.TakeID)
		}
		_ = csvWriter.Write([]string{
			fmt.Sprintf("%d", issue.ID), fmt.Sprintf("%d", issue.SegmentID), takeID,
			issue.IssueType, issue.Severity, issue.Status, issue.Note, issue.CreatedAt,
		})
	}
	csvWriter.Flush()
	return csvWriter.Error()
}

func buildReadme(projectTitle string, audioCount int, profile *store.ExportProfile) string {
	profileName := "No finishing profile"
	if profile != nil {
		profileName = profile.Name
	}
	return fmt.Sprintf(`Project: %s
Exported: %s
Finishing profile: %s
Segment audio files: %d

Contents
--------
audio/project-master.wav        - Stitched project master
audio/*.wav                     - One finished WAV per selected segment
project.json                    - Project structure and selected-take references
cast-bible.json                 - Cast profile definitions
pronunciation-dictionary.json   - Enabled pronunciation rules
qc-notes.csv                    - QC issues and annotations
render-metadata.json            - Render provenance and finishing settings
README.txt                      - This file

Audio is exported as 24 kHz, 16-bit, mono PCM WAV. Finishing-profile trimming,
normalization, leading/trailing padding, and inter-segment spacing are applied
before packaging. Missing referenced audio causes the export job to fail rather
than silently producing an incomplete deliverable.
`, projectTitle, time.Now().UTC().Format("2006-01-02 15:04:05 UTC"), profileName, audioCount)
}
