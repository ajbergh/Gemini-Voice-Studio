// Copyright 2026 ajbergh
// SPDX-License-Identifier: Apache-2.0

package audio

import (
	"encoding/binary"
	"math"
)

// FinishOptions describes deterministic processing for PCM16LE mono audio.
type FinishOptions struct {
	TrimSilence        bool
	SilenceThresholdDB float64
	LeadingSilenceMS   int
	TrailingSilenceMS  int
	NormalizePeakDB    float64
	SampleRate         int
}

// FinishPCM16 applies silence trimming, peak normalization, and edge padding.
// The input is never modified.
func FinishPCM16(pcm []byte, options FinishOptions) []byte {
	sampleRate := options.SampleRate
	if sampleRate <= 0 {
		sampleRate = DefaultSampleRate
	}

	result := append([]byte(nil), pcm...)
	if options.TrimSilence {
		result = append([]byte(nil), TrimPCM16Silence(result, options.SilenceThresholdDB)...)
	}
	if options.NormalizePeakDB != 0 {
		result = NormalizePCM16Peak(result, options.NormalizePeakDB)
	}
	if options.LeadingSilenceMS > 0 {
		result = append(PCM16Silence(options.LeadingSilenceMS, sampleRate), result...)
	}
	if options.TrailingSilenceMS > 0 {
		result = append(result, PCM16Silence(options.TrailingSilenceMS, sampleRate)...)
	}
	return result
}

// PCM16Silence returns zero-valued PCM16 mono samples for the requested duration.
func PCM16Silence(milliseconds, sampleRate int) []byte {
	if milliseconds <= 0 {
		return nil
	}
	if sampleRate <= 0 {
		sampleRate = DefaultSampleRate
	}
	samples := (milliseconds * sampleRate) / 1000
	return make([]byte, samples*2)
}

// TrimPCM16Silence removes leading and trailing samples at or below thresholdDB.
func TrimPCM16Silence(pcm []byte, thresholdDB float64) []byte {
	if len(pcm) < 2 {
		return pcm
	}
	threshold := math.Pow(10, thresholdDB/20) * 32768.0
	start := 0
	for start+1 < len(pcm) {
		sample := int16(binary.LittleEndian.Uint16(pcm[start : start+2]))
		if math.Abs(float64(sample)) > threshold {
			break
		}
		start += 2
	}
	end := len(pcm) - 2
	for end >= start {
		sample := int16(binary.LittleEndian.Uint16(pcm[end : end+2]))
		if math.Abs(float64(sample)) > threshold {
			break
		}
		end -= 2
	}
	if start > end {
		return nil
	}
	return pcm[start : end+2]
}

// NormalizePCM16Peak scales PCM16 samples to the requested peak dBFS.
func NormalizePCM16Peak(pcm []byte, targetDB float64) []byte {
	if len(pcm) < 2 {
		return append([]byte(nil), pcm...)
	}
	var peak float64
	for i := 0; i+1 < len(pcm); i += 2 {
		value := math.Abs(float64(int16(binary.LittleEndian.Uint16(pcm[i : i+2]))))
		if value > peak {
			peak = value
		}
	}
	if peak == 0 {
		return append([]byte(nil), pcm...)
	}
	target := math.Pow(10, targetDB/20) * 32768.0
	gain := target / peak
	out := make([]byte, len(pcm))
	for i := 0; i+1 < len(pcm); i += 2 {
		value := float64(int16(binary.LittleEndian.Uint16(pcm[i:i+2]))) * gain
		if value > 32767 {
			value = 32767
		} else if value < -32768 {
			value = -32768
		}
		binary.LittleEndian.PutUint16(out[i:i+2], uint16(int16(math.Round(value))))
	}
	return out
}

// EncodePCM16WAV wraps PCM16 audio in a RIFF/WAV header.
func EncodePCM16WAV(pcm []byte, sampleRate, channels int) []byte {
	if sampleRate <= 0 {
		sampleRate = DefaultSampleRate
	}
	if channels <= 0 {
		channels = DefaultChannels
	}
	const bitsPerSample = 16
	byteRate := sampleRate * channels * bitsPerSample / 8
	blockAlign := channels * bitsPerSample / 8
	dataSize := len(pcm)

	out := make([]byte, 44+dataSize)
	copy(out[0:4], "RIFF")
	binary.LittleEndian.PutUint32(out[4:8], uint32(36+dataSize))
	copy(out[8:12], "WAVE")
	copy(out[12:16], "fmt ")
	binary.LittleEndian.PutUint32(out[16:20], 16)
	binary.LittleEndian.PutUint16(out[20:22], 1)
	binary.LittleEndian.PutUint16(out[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(out[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(out[28:32], uint32(byteRate))
	binary.LittleEndian.PutUint16(out[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(out[34:36], bitsPerSample)
	copy(out[36:40], "data")
	binary.LittleEndian.PutUint32(out[40:44], uint32(dataSize))
	copy(out[44:], pcm)
	return out
}
