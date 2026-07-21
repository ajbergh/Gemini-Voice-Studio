// Copyright 2026 ajbergh
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ApplyEnvironment applies GVS_* environment variables over file/default config.
func ApplyEnvironment(cfg Config) (Config, error) {
	if value := strings.TrimSpace(os.Getenv("GVS_HOST")); value != "" {
		cfg.Host = value
	}
	if value := strings.TrimSpace(os.Getenv("GVS_PORT")); value != "" {
		port, err := strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			return cfg, fmt.Errorf("invalid GVS_PORT %q", value)
		}
		cfg.Port = port
	}
	if value := strings.TrimSpace(os.Getenv("GVS_DATA_DIR")); value != "" {
		cfg.DataDir = filepath.Clean(value)
		cfg.DBPath = filepath.Join(cfg.DataDir, "data.db")
		cfg.AudioCacheDir = filepath.Join(cfg.DataDir, "audio")
	}
	if value := strings.TrimSpace(os.Getenv("GVS_DB_PATH")); value != "" {
		cfg.DBPath = filepath.Clean(value)
	}
	if value := strings.TrimSpace(os.Getenv("GVS_AUDIO_DIR")); value != "" {
		cfg.AudioCacheDir = filepath.Clean(value)
	}
	if value := os.Getenv("GVS_PASSPHRASE"); value != "" {
		cfg.Passphrase = value
	}
	if value := strings.TrimSpace(os.Getenv("GVS_LOG_LEVEL")); value != "" {
		switch value {
		case "debug", "info", "warn", "error":
			cfg.LogLevel = value
		default:
			return cfg, fmt.Errorf("invalid GVS_LOG_LEVEL %q", value)
		}
	}
	if value := strings.TrimSpace(os.Getenv("GVS_OPEN_BROWSER")); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return cfg, fmt.Errorf("invalid GVS_OPEN_BROWSER %q", value)
		}
		cfg.OpenBrowser = parsed
	}
	return cfg, nil
}

// Validate checks the resolved runtime configuration.
func Validate(cfg Config) error {
	if strings.TrimSpace(cfg.Host) == "" {
		return fmt.Errorf("host is required")
	}
	if strings.ContainsAny(cfg.Host, " /\\") {
		return fmt.Errorf("host must be an IP address or hostname")
	}
	if ip := net.ParseIP(cfg.Host); ip == nil && cfg.Host != "localhost" {
		for _, label := range strings.Split(cfg.Host, ".") {
			if label == "" {
				return fmt.Errorf("host must be an IP address or hostname")
			}
		}
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	if strings.TrimSpace(cfg.DataDir) == "" {
		return fmt.Errorf("data directory is required")
	}
	if strings.TrimSpace(cfg.DBPath) == "" {
		return fmt.Errorf("database path is required")
	}
	if strings.TrimSpace(cfg.AudioCacheDir) == "" {
		return fmt.Errorf("audio directory is required")
	}
	return nil
}
