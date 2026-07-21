// Copyright 2025 ajbergh
// SPDX-License-Identifier: Apache-2.0

// Package config manages application runtime configuration.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Config holds the application runtime configuration.
type Config struct {
	Host          string `json:"host"`
	Port          int    `json:"port"`
	DBPath        string `json:"db_path"`
	Passphrase    string `json:"-"`
	LogLevel      string `json:"log_level"`
	OpenBrowser   bool   `json:"open_browser"`
	DataDir       string `json:"data_dir"`
	AudioCacheDir string `json:"audio_cache_dir"`
}

// DefaultConfig returns platform-aware defaults. Existing installations using
// the previous gemini-voice-library/audio_cache names are detected and retained
// so the product rename does not strand data.
func DefaultConfig() Config {
	dataDir := defaultDataDir()
	audioDir := filepath.Join(dataDir, "audio")
	legacyAudioDir := filepath.Join(dataDir, "audio_cache")
	if pathExists(legacyAudioDir) && !pathExists(audioDir) {
		audioDir = legacyAudioDir
	}
	return Config{
		Host:          "127.0.0.1",
		Port:          8080,
		DBPath:        filepath.Join(dataDir, "data.db"),
		LogLevel:      "info",
		OpenBrowser:   true,
		DataDir:       dataDir,
		AudioCacheDir: audioDir,
	}
}

// Load reads a JSON file over the defaults. Missing files are treated as an
// empty optional configuration source.
func Load(path string) (Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

// EnsureDataDir creates persistent directories with restrictive permissions.
func (c *Config) EnsureDataDir() error {
	if err := os.MkdirAll(c.DataDir, 0o700); err != nil {
		return err
	}
	return os.MkdirAll(c.AudioCacheDir, 0o700)
}

func defaultDataDir() string {
	newDir, legacyDir := platformDataDirs()
	if pathExists(legacyDir) && !pathExists(newDir) {
		return legacyDir
	}
	return newDir
}

func platformDataDirs() (newDir, legacyDir string) {
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("APPDATA")
		if base == "" {
			home, _ := os.UserHomeDir()
			base = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(base, "gemini-voice-studio"), filepath.Join(base, "gemini-voice-library")
	case "darwin":
		home, _ := os.UserHomeDir()
		base := filepath.Join(home, "Library", "Application Support")
		return filepath.Join(base, "gemini-voice-studio"), filepath.Join(base, "gemini-voice-library")
	default:
		base := os.Getenv("XDG_DATA_HOME")
		if base == "" {
			home, _ := os.UserHomeDir()
			base = filepath.Join(home, ".local", "share")
		}
		return filepath.Join(base, "gemini-voice-studio"), filepath.Join(base, "gemini-voice-library")
	}
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
