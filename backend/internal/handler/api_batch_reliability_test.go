// Copyright 2026 ajbergh
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"path/filepath"
	"testing"

	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/store"
)

func TestResolveBatchConcurrency(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "batch-concurrency.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer st.Close()
	handler := &BatchHandler{Store: st}

	if got := handler.resolveBatchConcurrency(0); got != 2 {
		t.Fatalf("default concurrency = %d, want 2", got)
	}
	if got := handler.resolveBatchConcurrency(99); got != maxBatchConcurrency {
		t.Fatalf("clamped concurrency = %d, want %d", got, maxBatchConcurrency)
	}
	if got := handler.resolveBatchConcurrency(-1); got != 2 {
		t.Fatalf("negative concurrency = %d, want default 2", got)
	}
}

func TestModelCompatibleRejectsUnknownGeminiModel(t *testing.T) {
	if modelCompatibleWithProvider("gemini", "made-up-model") {
		t.Fatal("unknown Gemini model should not be considered compatible")
	}
	if !modelCompatibleWithProvider("gemini", "gemini-2.5-pro-preview-tts") {
		t.Fatal("registered Gemini Pro TTS model should be compatible")
	}
}
