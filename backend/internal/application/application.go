// Copyright 2026 ajbergh
// SPDX-License-Identifier: Apache-2.0

// Package application owns the shared backend runtime used by the browser
// server and the Windows-native Wails shell.
package application

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/config"
	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/crypto"
	fe "github.com/ajbergh/gemini-voice-gen-tts/backend/internal/embed"
	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/server"
	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/store"
)

// Application contains the initialized backend services and HTTP handler.
// The caller owns the lifecycle and must call Close when it exits.
type Application struct {
	Store   *store.Store
	Handler http.Handler
}

// New initializes the backend using the address represented by cfg.Host and
// cfg.Port. It is used by the standalone HTTP server.
func New(cfg config.Config) (*Application, error) {
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	return NewAt(cfg, addr)
}

// NewAt initializes the backend while allowing the caller to choose the HTTP
// listener address. Wails uses a loopback listener on port 0 so each native
// instance gets a private, collision-free backend endpoint.
func NewAt(cfg config.Config, addr string) (*Application, error) {
	if err := cfg.EnsureDataDir(); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}

	cryptoKey, err := crypto.DeriveKey(cfg.Passphrase, cfg.AudioCacheDir)
	if err != nil {
		return nil, fmt.Errorf("derive encryption key: %w", err)
	}
	st, err := store.New(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	frontendFS := fe.FrontendFS()
	srv := server.New(addr, st, cryptoKey, frontendFS, cfg.AudioCacheDir)
	return &Application{Store: st, Handler: srv.Handler()}, nil
}

// NewHTTPServer creates an HTTP server with the same production timeouts used
// by the standalone server. The Wails shell supplies its own loopback listener.
func (a *Application) NewHTTPServer(addr string) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           a.Handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      0,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
}

// Close releases persistent backend resources.
func (a *Application) Close() error {
	if a == nil || a.Store == nil {
		return nil
	}
	return a.Store.Close()
}
