// Copyright 2025 ajbergh
// SPDX-License-Identifier: Apache-2.0

// Package main is the entry point for the Gemini Voice Studio server.
//
// It parses CLI flags, loads platform-aware defaults, derives the API-key
// encryption key, opens SQLite, embeds the frontend SPA, and starts the HTTP
// server with graceful shutdown on SIGINT/SIGTERM.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/config"
	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/crypto"
	fe "github.com/ajbergh/gemini-voice-gen-tts/backend/internal/embed"
	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/server"
	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/store"
)

var (
	version   = "dev"
	commitSHA = "unknown"
	buildDate = "unknown"
)

// main wires CLI configuration, persistent storage, frontend assets, and HTTP lifecycle.
func main() {
	port := flag.Int("port", 0, "HTTP server port (default: 8080)")
	dataDir := flag.String("data-dir", "", "Persistent application data directory")
	dbPath := flag.String("db", "", "SQLite database path")
	audioDir := flag.String("audio-dir", "", "Persistent generated-audio directory")
	passphrase := flag.String("passphrase", "", "Encryption passphrase (uses machine identity if empty)")
	logLevel := flag.String("log-level", "", "Log level: debug, info, warn, error")
	openBrowser := flag.Bool("open", true, "Open browser on startup")
	showVersion := flag.Bool("version", false, "Print version information and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("Gemini Voice Studio %s (%s, built %s)\n", version, commitSHA, buildDate)
		return
	}

	cfg := config.DefaultConfig()
	if *dataDir != "" {
		cfg.DataDir = filepath.Clean(*dataDir)
		cfg.DBPath = filepath.Join(cfg.DataDir, "data.db")
		cfg.AudioCacheDir = filepath.Join(cfg.DataDir, "audio")
	}
	if *port != 0 {
		cfg.Port = *port
	}
	if *dbPath != "" {
		cfg.DBPath = filepath.Clean(*dbPath)
		if *dataDir == "" && *audioDir == "" {
			cfg.DataDir = filepath.Dir(cfg.DBPath)
			cfg.AudioCacheDir = filepath.Join(cfg.DataDir, "audio")
		}
	}
	if *audioDir != "" {
		cfg.AudioCacheDir = filepath.Clean(*audioDir)
	}
	if *passphrase != "" {
		cfg.Passphrase = *passphrase
	}
	if *logLevel != "" {
		cfg.LogLevel = *logLevel
	}
	cfg.OpenBrowser = *openBrowser

	var level slog.Level
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	if err := cfg.EnsureDataDir(); err != nil {
		slog.Error("failed to create data directory", "error", err)
		os.Exit(1)
	}

	cryptoKey, err := crypto.DeriveKey(cfg.Passphrase)
	if err != nil {
		slog.Error("failed to derive encryption key", "error", err)
		os.Exit(1)
	}

	st, err := store.New(cfg.DBPath)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer st.Close()

	frontendFS := fe.FrontendFS()
	addr := fmt.Sprintf("127.0.0.1:%d", cfg.Port)
	srv := server.New(addr, st, cryptoKey, frontendFS, cfg.AudioCacheDir)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      0, // streaming endpoints manage their own provider timeouts
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		slog.Info("shutting down server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			slog.Warn("graceful shutdown timed out", "error", err)
		}
	}()

	url := fmt.Sprintf("http://localhost:%d", cfg.Port)
	if cfg.OpenBrowser {
		go func() {
			time.Sleep(500 * time.Millisecond)
			openURL(url)
		}()
	}

	slog.Info("starting server",
		"addr", url,
		"db", cfg.DBPath,
		"audio_dir", cfg.AudioCacheDir,
		"version", version,
		"commit", commitSHA,
	)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
	slog.Info("server stopped")
}

// openURL opens a URL in the default browser.
func openURL(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		slog.Warn("failed to open browser", "error", err)
	}
}
