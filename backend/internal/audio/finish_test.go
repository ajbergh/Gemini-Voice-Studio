// Copyright 2026 ajbergh
// SPDX-License-Identifier: Apache-2.0

package audio

import (
	"encoding/binary"
	"testing"
)

func pcmSamples(values ...int16) []byte {
	out := make([]byte, len(values)*2)
	for index, value := range values {
		binary.LittleEndian.PutUint16(out[index*2:index*2+2], uint16(value))
	}
	return out
}

func TestTrimPCM16Silence(t *testing.T) {
	input := pcmSamples(0, 0, 1000, -1200, 0)
	trimmed := TrimPCM16Silence(input, -40)
	if len(trimmed) != 4 {
		t.Fatalf("trimmed length = %d, want 4", len(trimmed))
	}
	if got := int16(binary.LittleEndian.Uint16(trimmed[:2])); got != 1000 {
		t.Fatalf("first sample = %d, want 1000", got)
	}
}

func TestNormalizePCM16Peak(t *testing.T) {
	input := pcmSamples(1000, -2000)
	normalized := NormalizePCM16Peak(input, -6.0206) // approximately half scale
	peak := int16(binary.LittleEndian.Uint16(normalized[2:4]))
	if peak > -16380 || peak < -16390 {
		t.Fatalf("peak sample = %d, want approximately -16384", peak)
	}
}

func TestFinishPCM16Padding(t *testing.T) {
	input := pcmSamples(1000)
	finished := FinishPCM16(input, FinishOptions{
		LeadingSilenceMS:  1,
		TrailingSilenceMS: 1,
		SampleRate:        1000,
	})
	if len(finished) != 6 {
		t.Fatalf("finished length = %d, want 6", len(finished))
	}
}

func TestEncodePCM16WAV(t *testing.T) {
	wav := EncodePCM16WAV(pcmSamples(1, 2), 24000, 1)
	if string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		t.Fatalf("invalid WAV header")
	}
	if got := binary.LittleEndian.Uint32(wav[40:44]); got != 4 {
		t.Fatalf("data size = %d, want 4", got)
	}
}
