// Copyright 2025 ajbergh
// SPDX-License-Identifier: Apache-2.0

// Package handler — api_providers.go implements the provider registry
// endpoint at GET /api/providers.
package handler

import (
	"net/http"

	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/gemini"
	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/store"
)

// ProviderCapabilities describes what a TTS provider supports.
type ProviderCapabilities struct {
	SingleSpeakerTTS  bool `json:"single_speaker_tts"`
	MultiSpeakerTTS   bool `json:"multi_speaker_tts"`
	Streaming         bool `json:"streaming"`
	LanguageSelection bool `json:"language_selection"`
	VoiceList         bool `json:"voice_list"`
	PCMOutput         bool `json:"pcm_output"`
}

// ProviderModel describes a model offered by a provider.
type ProviderModel struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	IsDefault   bool   `json:"is_default,omitempty"`
	Streaming   bool   `json:"streaming,omitempty"`
	Notes       string `json:"notes,omitempty"`
}

// ProviderVoice describes a voice offered by a provider.
type ProviderVoice struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// ProviderInfo describes a TTS provider in the registry.
type ProviderInfo struct {
	ID           string               `json:"id"`
	DisplayName  string               `json:"display_name"`
	Capabilities ProviderCapabilities `json:"capabilities"`
	Models       []ProviderModel      `json:"models"`
	Voices       []ProviderVoice      `json:"voices"`
	DefaultModel string               `json:"default_model"`
	KeyProvider  string               `json:"key_provider"`
}

// ProvidersHandler handles /api/providers endpoints.
type ProvidersHandler struct {
	Store       *store.Store
	KeysHandler *KeysHandler
}

func registeredGeminiModels() []ProviderModel {
	models := gemini.SupportedTTSModels()
	out := make([]ProviderModel, 0, len(models))
	for _, model := range models {
		out = append(out, ProviderModel{
			ID:          model.ID,
			DisplayName: model.Name,
			IsDefault:   model.ID == "gemini-3.1-flash-tts-preview",
			Streaming:   model.Streaming,
			Notes:       model.Quality,
		})
	}
	return out
}

// registry is the static provider list. Model data is sourced from the Gemini
// package so backend validation, project settings, and API discovery agree.
var registry = []ProviderInfo{
	{
		ID:          "gemini",
		DisplayName: "Google Gemini",
		KeyProvider: "gemini",
		Capabilities: ProviderCapabilities{
			SingleSpeakerTTS:  true,
			MultiSpeakerTTS:   true,
			Streaming:         true,
			LanguageSelection: true,
			VoiceList:         true,
			PCMOutput:         true,
		},
		DefaultModel: "gemini-3.1-flash-tts-preview",
		Models:       registeredGeminiModels(),
		Voices: []ProviderVoice{
			{ID: "Zephyr", DisplayName: "Zephyr"},
			{ID: "Puck", DisplayName: "Puck"},
			{ID: "Charon", DisplayName: "Charon"},
			{ID: "Kore", DisplayName: "Kore"},
			{ID: "Fenrir", DisplayName: "Fenrir"},
			{ID: "Aoede", DisplayName: "Aoede"},
			{ID: "Leda", DisplayName: "Leda"},
			{ID: "Orus", DisplayName: "Orus"},
			{ID: "Perseus", DisplayName: "Perseus"},
			{ID: "Achernar", DisplayName: "Achernar"},
			{ID: "Alnilam", DisplayName: "Alnilam"},
			{ID: "Schedar", DisplayName: "Schedar"},
			{ID: "Gacrux", DisplayName: "Gacrux"},
			{ID: "Pulcherrima", DisplayName: "Pulcherrima"},
			{ID: "Achird", DisplayName: "Achird"},
			{ID: "Zubenelgenubi", DisplayName: "Zubenelgenubi"},
			{ID: "Vindemiatrix", DisplayName: "Vindemiatrix"},
			{ID: "Sadachbia", DisplayName: "Sadachbia"},
			{ID: "Sadaltager", DisplayName: "Sadaltager"},
			{ID: "Sulafat", DisplayName: "Sulafat"},
			{ID: "Umbriel", DisplayName: "Umbriel"},
			{ID: "Algieba", DisplayName: "Algieba"},
			{ID: "Despina", DisplayName: "Despina"},
			{ID: "Erinome", DisplayName: "Erinome"},
			{ID: "Algenib", DisplayName: "Algenib"},
			{ID: "Rasalgethi", DisplayName: "Rasalgethi"},
			{ID: "Laomedeia", DisplayName: "Laomedeia"},
			{ID: "Acrab", DisplayName: "Acrab"},
			{ID: "Iocaste", DisplayName: "Iocaste"},
			{ID: "Spica", DisplayName: "Spica"},
		},
	},
}

// ListProviders returns the provider registry with key-configured status.
func (h *ProvidersHandler) ListProviders(w http.ResponseWriter, r *http.Request) {
	type providerResponse struct {
		ProviderInfo
		KeyConfigured bool `json:"key_configured"`
	}

	out := make([]providerResponse, len(registry))
	for i, provider := range registry {
		_, keyErr := h.KeysHandler.GetDecryptedKey(provider.KeyProvider)
		out[i] = providerResponse{ProviderInfo: provider, KeyConfigured: keyErr == nil}
	}
	writeJSON(w, http.StatusOK, out)
}
