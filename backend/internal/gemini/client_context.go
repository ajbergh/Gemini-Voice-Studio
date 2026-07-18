// Copyright 2026 ajbergh
// SPDX-License-Identifier: Apache-2.0

package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GenerateTTSContext performs TTS generation with request cancellation and
// cancellable retry backoff. This is the production render path used by jobs.
func (c *Client) GenerateTTSContext(ctx context.Context, text, voiceName, systemInstruction, languageCode, model string) (string, error) {
	if err := ValidateTTSModel(model); err != nil {
		return "", err
	}
	spokenText := text
	if systemInstruction != "" {
		directions := systemInstruction
		for _, marker := range []string{"## Transcript", "## TRANSCRIPT", "##Transcript"} {
			if index := strings.Index(strings.ToLower(directions), strings.ToLower(marker)); index >= 0 {
				directions = strings.TrimRight(directions[:index], "\n\r ")
				break
			}
		}
		spokenText = directions + "\n\n## Transcript\n" + text
	}

	speechConfig := map[string]any{
		"voiceConfig": map[string]any{
			"prebuiltVoiceConfig": map[string]any{"voiceName": voiceName},
		},
	}
	if languageCode != "" {
		speechConfig["languageCode"] = languageCode
	}
	requestBody := map[string]any{
		"contents": []map[string]any{{"parts": []map[string]any{{"text": spokenText}}}},
		"generationConfig": map[string]any{
			"responseModalities": []string{"AUDIO"},
			"speechConfig":       speechConfig,
		},
	}
	data, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}
	endpoint := fmt.Sprintf("%s/models/%s:generateContent?key=%s", baseURL, resolveTTSModel(model), c.apiKey)

	var lastErr error
	for attempt := 0; attempt < maxTTSRetries; attempt++ {
		if attempt > 0 {
			if err := waitForRetry(ctx, time.Duration(attempt)*time.Second); err != nil {
				return "", err
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
		if err != nil {
			return "", fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.httpClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			lastErr = fmt.Errorf("http request: %w", err)
			continue
		}
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("read response: %w", readErr)
			continue
		}
		if (resp.StatusCode == http.StatusInternalServerError || resp.StatusCode == http.StatusServiceUnavailable) && attempt < maxTTSRetries-1 {
			lastErr = fmt.Errorf("gemini TTS API error (status %d): %s", resp.StatusCode, string(body))
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("gemini TTS API error (status %d): %s", resp.StatusCode, string(body))
		}
		audioBase64, diagnostic, parseErr := parseTTSAudioResponse(body)
		if parseErr != nil {
			return "", fmt.Errorf("parse response: %w", parseErr)
		}
		if audioBase64 == "" {
			lastErr = fmt.Errorf("Gemini returned no audio data: %s", diagnostic)
			if attempt < maxTTSRetries-1 {
				continue
			}
			return "", lastErr
		}
		return audioBase64, nil
	}
	return "", fmt.Errorf("TTS failed after %d attempts: %w", maxTTSRetries, lastErr)
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
