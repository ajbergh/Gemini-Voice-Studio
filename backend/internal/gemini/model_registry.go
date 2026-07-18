// Copyright 2026 ajbergh
// SPDX-License-Identifier: Apache-2.0

package gemini

import (
	"fmt"
	"sort"
	"strings"
)

// TTSModel describes a supported Gemini text-to-speech model.
type TTSModel struct {
	ID        string
	Name      string
	Streaming bool
	Quality   string
}

var ttsModelRegistry = map[string]TTSModel{
	"gemini-3.1-flash-tts-preview": {
		ID:        "gemini-3.1-flash-tts-preview",
		Name:      "Gemini 3.1 Flash TTS",
		Streaming: true,
		Quality:   "Low-latency expressive generation",
	},
	"gemini-2.5-flash-preview-tts": {
		ID:        "gemini-2.5-flash-preview-tts",
		Name:      "Gemini 2.5 Flash TTS",
		Streaming: false,
		Quality:   "Cost-efficient high-volume generation",
	},
	"gemini-2.5-pro-preview-tts": {
		ID:        "gemini-2.5-pro-preview-tts",
		Name:      "Gemini 2.5 Pro TTS",
		Streaming: false,
		Quality:   "Highest-fidelity long-form generation",
	},
}

func init() {
	// Keep the legacy client's internal allowlist synchronized while the HTTP
	// transport is migrated to the Interactions API.
	for model := range ttsModelRegistry {
		allowedTTSModels[model] = true
	}
}

// ValidateTTSModel rejects unknown model identifiers instead of allowing an
// invisible substitution that would invalidate render provenance.
func ValidateTTSModel(model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil
	}
	if _, ok := ttsModelRegistry[model]; !ok {
		return fmt.Errorf("unsupported TTS model %q", model)
	}
	return nil
}

// GetTTSModel returns model metadata after validation.
func GetTTSModel(model string) (TTSModel, bool) {
	entry, ok := ttsModelRegistry[strings.TrimSpace(model)]
	return entry, ok
}

// SupportedTTSModels returns a stable copy of the model catalogue.
func SupportedTTSModels() []TTSModel {
	out := make([]TTSModel, 0, len(ttsModelRegistry))
	for _, model := range ttsModelRegistry {
		out = append(out, model)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
