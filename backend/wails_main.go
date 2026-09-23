//go:build windows

// Copyright 2026 ajbergh
// SPDX-License-Identifier: Apache-2.0

// The Windows-native entrypoint is intentionally separate from cmd/server.
// Wails builds the backend module root, while the existing portable release
// continues to build ./cmd/server.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/application"
	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/buildinfo"
	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/config"
	"github.com/wailsapp/wails/v2"
	wailsassetserver "github.com/wailsapp/wails/v2/pkg/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options"
	assetserveropts "github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

func main() {
	cfg, err := nativeConfig()
	if err != nil {
		fatalNative("configuration error", err)
	}
	configureNativeLogging(cfg.LogLevel)

	app, err := application.NewAt(cfg, "127.0.0.1:0")
	if err != nil {
		fatalNative("backend initialization failed", err)
	}
	defer app.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fatalNative("backend listener failed", err)
	}
	defer listener.Close()

	addr := listener.Addr().String()
	httpServer := app.NewHTTPServer(addr)
	serverErr := make(chan error, 1)
	go func() {
		slog.Info("starting native backend", "addr", addr, "version", buildinfo.Version, "commit", buildinfo.Commit)
		serverErr <- httpServer.Serve(listener)
	}()

	shutdown := func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			slog.Warn("native backend shutdown timed out", "error", err)
		}
	}

	proxyURL := "http://" + addr
	if err := wails.Run(&options.App{
		Title:     "Gemini Voice Studio",
		Width:     1440,
		Height:    900,
		MinWidth:  960,
		MinHeight: 640,
		AssetServer: &assetserveropts.Options{
			Handler: wailsassetserver.NewProxyServer(proxyURL),
		},
		Windows: &windows.Options{
			WebviewUserDataPath: filepath.Join(cfg.DataDir, "webview"),
			Theme:               windows.SystemDefault,
		},
		OnShutdown: func(context.Context) {
			shutdown()
		},
	}); err != nil {
		shutdown()
		fatalNative("Wails exited with an error", err)
	}

	select {
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			fatalNative("native backend stopped unexpectedly", err)
		}
	default:
	}
}

func nativeConfig() (config.Config, error) {
	cfg := config.DefaultConfig()
	if path := os.Getenv("GVS_CONFIG"); path != "" {
		loaded, err := config.Load(filepath.Clean(path))
		if err != nil {
			return cfg, err
		}
		cfg = loaded
	}
	var err error
	cfg, err = config.ApplyEnvironment(cfg)
	if err != nil {
		return cfg, err
	}
	// Native mode always uses an ephemeral loopback listener. This prevents a
	// GUI launch from exposing the API or colliding with a browser-server port.
	cfg.Host = "127.0.0.1"
	cfg.Port = 0
	cfg.OpenBrowser = false
	return cfg, nil
}

func configureNativeLogging(levelName string) {
	var level slog.Level
	switch levelName {
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
}

func fatalNative(message string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", message, err)
	os.Exit(1)
}
