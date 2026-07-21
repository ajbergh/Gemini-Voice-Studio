// Copyright 2025 ajbergh
// SPDX-License-Identifier: Apache-2.0

// Package handler - api_cache.go exposes media-storage statistics and safe
// garbage collection. Files referenced by database records are durable assets
// and are never removed by the cache-clear operation.
package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/store"
)

// CacheHandler handles /api/cache endpoints for media storage management.
type CacheHandler struct {
	Store         *store.Store
	AudioCacheDir string
}

type mediaStats struct {
	TotalSize        int64  `json:"total_size"`
	FileCount        int    `json:"file_count"`
	ProtectedSize    int64  `json:"protected_size"`
	ProtectedFiles   int    `json:"protected_files"`
	ReclaimableSize  int64  `json:"reclaimable_size"`
	ReclaimableFiles int    `json:"reclaimable_files"`
	CacheDir         string `json:"cache_dir"`
}

// GetCacheStats reports durable/referenced and reclaimable media separately.
func (h *CacheHandler) GetCacheStats(w http.ResponseWriter, r *http.Request) {
	referenced, err := h.referencedFiles()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to inspect referenced media")
		return
	}
	stats := mediaStats{CacheDir: h.AudioCacheDir}
	entries, err := os.ReadDir(h.AudioCacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusOK, stats)
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to inspect media directory")
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		stats.TotalSize += info.Size()
		stats.FileCount++
		if referenced[entry.Name()] {
			stats.ProtectedSize += info.Size()
			stats.ProtectedFiles++
		} else {
			stats.ReclaimableSize += info.Size()
			stats.ReclaimableFiles++
		}
	}
	writeJSON(w, http.StatusOK, stats)
}

// ClearCache removes only unreferenced files. Project takes, history audio,
// preset samples, generated artwork, and encryption metadata remain intact.
func (h *CacheHandler) ClearCache(w http.ResponseWriter, r *http.Request) {
	referenced, err := h.referencedFiles()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to inspect referenced media")
		return
	}
	entries, err := os.ReadDir(h.AudioCacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusOK, map[string]any{"status": "cleared", "removed": 0, "protected": 0})
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to inspect media directory")
		return
	}
	removed := 0
	protected := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if referenced[entry.Name()] {
			protected++
			continue
		}
		path, ok := safeCachePath(h.AudioCacheDir, entry.Name())
		if !ok {
			continue
		}
		if os.Remove(path) == nil {
			removed++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "cleared", "removed": removed, "protected": protected,
	})
}

// referencedFiles returns basenames for every durable media file.
func (h *CacheHandler) referencedFiles() (map[string]bool, error) {
	refs := map[string]bool{
		"encryption.salt":                 true,
		"encryption.salt.restore-pending": true,
	}
	if h.Store == nil {
		return refs, nil
	}
	rows, err := h.Store.DB().Query(`
		SELECT audio_path FROM history WHERE audio_path IS NOT NULL AND audio_path <> ''
		UNION ALL
		SELECT audio_path FROM custom_presets WHERE audio_path IS NOT NULL AND audio_path <> ''
		UNION ALL
		SELECT audio_path FROM segment_takes WHERE audio_path IS NOT NULL AND audio_path <> ''
	`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if name := filepath.Base(strings.TrimSpace(path)); name != "" && name != "." {
			refs[name] = true
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()

	metaRows, err := h.Store.DB().Query(`SELECT metadata_json FROM custom_presets WHERE metadata_json IS NOT NULL AND metadata_json <> ''`)
	if err != nil {
		return nil, err
	}
	defer metaRows.Close()
	for metaRows.Next() {
		var raw string
		if err := metaRows.Scan(&raw); err != nil {
			return nil, err
		}
		var metadata struct {
			Headshot *struct {
				Path string `json:"path"`
			} `json:"headshot"`
		}
		if json.Unmarshal([]byte(raw), &metadata) == nil && metadata.Headshot != nil {
			if name := filepath.Base(strings.TrimSpace(metadata.Headshot.Path)); name != "" && name != "." {
				refs[name] = true
			}
		}
	}
	return refs, metaRows.Err()
}
