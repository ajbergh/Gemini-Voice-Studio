// Copyright 2025 ajbergh
// SPDX-License-Identifier: Apache-2.0

// Package main is the entry point for Gemini Voice Studio.
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

	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/buildinfo"
	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/config"
	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/crypto"
	fe "github.com/ajbergh/gemini-voice-gen-tts/backend/internal/embed"
	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/server"
	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/store"
)

func main() {
	configPath := flag.String("config", "", "Optional JSON configuration file")
	port := flag.Int("port", 0, "HTTP server port")
	dataDir := flag.String("data-dir", "", "Persistent application data directory")
	dbPath := flag.String("db", "", "SQLite database path")
	audioDir := flag.String("audio-dir", "", "Persistent generated-audio directory")
	passphrase := flag.String("passphrase", "", "Encryption passphrase")
	logLevel := flag.String("log-level", "", "Log level: debug, info, warn, error")
	openBrowser := flag.Bool("open", true, "Open browser on startup")
	showVersion := flag.Bool("version", false, "Print version information and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("Gemini Voice Studio %s (%s, built %s)\n", buildinfo.Version, buildinfo.Commit, buildinfo.Date)
		return
	}

	resolvedConfigPath := *configPath
	if resolvedConfigPath == "" {
		resolvedConfigPath = os.Getenv("GVS_CONFIG")
	}
	cfg := config.DefaultConfig()
	if resolvedConfigPath != "" {
		loaded, err := config.Load(filepath.Clean(resolvedConfigPath))
		if err != nil {
			fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
			os.Exit(2)
		}
		cfg = loaded
	}
	var err error
	cfg, err = config.ApplyEnvironment(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "environment configuration error: %v\n", err)
		os.Exit(2)
	}

	provided := map[string]bool{}
	flag.Visit(func(current *flag.Flag) { provided[current.Name] = true })
	if provided["data-dir"] {
		cfg.DataDir = filepath.Clean(*dataDir)
		cfg.DBPath = filepath.Join(cfg.DataDir, "data.db")
		cfg.AudioCacheDir = filepath.Join(cfg.DataDir, "audio")
	}
	if provided["port"] {
		cfg.Port = *port
	}
	if provided["db"] {
		cfg.DBPath = filepath.Clean(*dbPath)
		if !provided["data-dir"] && !provided["audio-dir"] {
			cfg.DataDir = filepath.Dir(cfg.DBPath)
			cfg.AudioCacheDir = filepath.Join(cfg.DataDir, "audio")
		}
	}
	if provided["audio-dir"] {
		cfg.AudioCacheDir = filepath.Clean(*audioDir)
	}
	if provided["passphrase"] {
		cfg.Passphrase = *passphrase
	}
	if provided["log-level"] {
		cfg.LogLevel = *logLevel
	}
	if provided["open"] {
		cfg.OpenBrowser = *openBrowser
	}
	if err := config.Validate(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "invalid configuration: %v\n", err)
		os.Exit(2)
	}

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

	// The installation salt is stored beside durable generated media so portable
	// backups capture both encrypted rows and the KDF metadata required to unlock them.
	cryptoKey, err := crypto.DeriveKey(cfg.Passphrase, cfg.AudioCacheDir)
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
		Addr: addr, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout: 30 * time.Second, WriteTimeout: 0,
		IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20,
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
		"addr", url, "db", cfg.DBPath, "audio_dir", cfg.AudioCacheDir,
		"version", buildinfo.Version, "commit", buildinfo.Commit,
		"schema", st.SchemaVersion(),
	)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
	slog.Info("server stopped")
}

func openURL(url string) {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		command = exec.Command("open", url)
	default:
		command = exec.Command("xdg-open", url)
	}
	if err := command.Start(); err != nil {
		slog.Warn("failed to open browser", "error", err)
	}
}
