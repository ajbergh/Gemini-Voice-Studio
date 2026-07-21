// Copyright 2026 ajbergh
// SPDX-License-Identifier: Apache-2.0

package gemini

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GenerateTTSContext performs single-speaker TTS with request cancellation and
// cancellable retry backoff.
func (c *Client) GenerateTTSContext(ctx context.Context, text, voiceName, systemInstruction, languageCode, model string) (string, error) {
	if err := ValidateTTSModel(model); err != nil {
		return "", err
	}
	speechConfig := map[string]any{
		"voiceConfig": map[string]any{
			"prebuiltVoiceConfig": map[string]any{"voiceName": voiceName},
		},
	}
	if languageCode != "" {
		speechConfig["languageCode"] = languageCode
	}
	data, err := marshalAudioRequest(spokenText(text, systemInstruction), speechConfig)
	if err != nil {
		return "", err
	}
	endpoint := fmt.Sprintf("%s/models/%s:generateContent?key=%s", baseURL, resolveTTSModel(model), c.apiKey)
	return c.executeAudioRequest(ctx, endpoint, data)
}

// GenerateMultiSpeakerTTSContext performs multi-speaker TTS with cancellation.
func (c *Client) GenerateMultiSpeakerTTSContext(ctx context.Context, text string, speakers []SpeakerConfig, languageCode, model string) (string, error) {
	if err := ValidateTTSModel(model); err != nil {
		return "", err
	}
	configs := make([]map[string]any, len(speakers))
	for index, speaker := range speakers {
		configs[index] = map[string]any{
			"speaker": speaker.Speaker,
			"voiceConfig": map[string]any{
				"prebuiltVoiceConfig": map[string]any{"voiceName": speaker.VoiceName},
			},
		}
	}
	speechConfig := map[string]any{
		"multiSpeakerVoiceConfig": map[string]any{"speakerVoiceConfigs": configs},
	}
	if languageCode != "" {
		speechConfig["languageCode"] = languageCode
	}
	data, err := marshalAudioRequest(text, speechConfig)
	if err != nil {
		return "", err
	}
	endpoint := fmt.Sprintf("%s/models/%s:generateContent?key=%s", baseURL, resolveTTSModel(model), c.apiKey)
	return c.executeAudioRequest(ctx, endpoint, data)
}

// GenerateTTSStreamContext streams audio chunks while honoring cancellation.
func (c *Client) GenerateTTSStreamContext(ctx context.Context, text, voiceName, systemInstruction, languageCode, model string, chunks chan<- StreamTTSChunk) error {
	defer close(chunks)
	if err := ValidateTTSModel(model); err != nil {
		return err
	}
	effectiveModel := resolveTTSModel(model)
	metadata, ok := GetTTSModel(effectiveModel)
	if !ok || !metadata.Streaming {
		return fmt.Errorf("TTS model %q does not support streaming", effectiveModel)
	}
	speechConfig := map[string]any{
		"voiceConfig": map[string]any{
			"prebuiltVoiceConfig": map[string]any{"voiceName": voiceName},
		},
	}
	if languageCode != "" {
		speechConfig["languageCode"] = languageCode
	}
	data, err := marshalAudioRequest(spokenText(text, systemInstruction), speechConfig)
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse&key=%s", baseURL, effectiveModel, c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create streaming request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("streaming request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return fmt.Errorf("gemini streaming TTS API error (status %d): %s", resp.StatusCode, string(body))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)
	index := 0
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		audioBase64, _, parseErr := parseTTSAudioResponse([]byte(payload))
		if parseErr != nil || audioBase64 == "" {
			continue
		}
		select {
		case chunks <- StreamTTSChunk{AudioBase64: audioBase64, Index: index}:
			index++
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("reading stream: %w", err)
	}
	select {
	case chunks <- StreamTTSChunk{Done: true, Index: index}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) executeAudioRequest(ctx context.Context, endpoint string, data []byte) (string, error) {
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
		if isRetryableTTSStatus(resp.StatusCode) && attempt < maxTTSRetries-1 {
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

func marshalAudioRequest(text string, speechConfig map[string]any) ([]byte, error) {
	requestBody := map[string]any{
		"contents": []map[string]any{{"parts": []map[string]any{{"text": text}}}},
		"generationConfig": map[string]any{
			"responseModalities": []string{"AUDIO"},
			"speechConfig":       speechConfig,
		},
	}
	data, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	return data, nil
}

func spokenText(text, systemInstruction string) string {
	if systemInstruction == "" {
		return text
	}
	directions := systemInstruction
	for _, marker := range []string{"## Transcript", "## TRANSCRIPT", "##Transcript"} {
		if index := strings.Index(strings.ToLower(directions), strings.ToLower(marker)); index >= 0 {
			directions = strings.TrimRight(directions[:index], "\n\r ")
			break
		}
	}
	return directions + "\n\n## Transcript\n" + text
}

func isRetryableTTSStatus(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusInternalServerError || status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
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
