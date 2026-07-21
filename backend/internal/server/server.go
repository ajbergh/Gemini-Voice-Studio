// Copyright 2025 ajbergh
// SPDX-License-Identifier: Apache-2.0

// Package server assembles the HTTP server with all handlers, routes,
// middleware, and the SPA fallback handler for the embedded frontend.
package server

import (
	"fmt"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/handler"
	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/store"
)

// Server holds the HTTP server and its dependencies.
type Server struct {
	Mux  *http.ServeMux
	Addr string
}

// New creates a new Server with all routes and middleware configured.
func New(addr string, st *store.Store, cryptoKey []byte, frontendFS fs.FS, audioCacheDir string) *Server {
	mux := http.NewServeMux()

	configH := &handler.ConfigHandler{Store: st}
	keysH := &handler.KeysHandler{Store: st, CryptoKey: cryptoKey}
	historyH := &handler.HistoryHandler{Store: st, AudioCacheDir: audioCacheDir}
	jobsH := &handler.JobsHandler{Store: st}
	projectsH := &handler.ProjectsHandler{Store: st}
	takesH := &handler.TakesHandler{Store: st, AudioCacheDir: audioCacheDir}
	progressH := handler.NewProgressHub(st)
	voicesH := &handler.VoicesHandler{Store: st, KeysHandler: keysH, AudioCacheDir: audioCacheDir, ProgressHub: progressH}
	presetsH := &handler.PresetsHandler{Store: st, AudioCacheDir: audioCacheDir, KeysHandler: keysH}
	favoritesH := &handler.FavoritesHandler{Store: st}
	cacheH := &handler.CacheHandler{Store: st, AudioCacheDir: audioCacheDir}
	backupH := &handler.BackupHandler{Store: st, AudioCacheDir: audioCacheDir}
	batchH := &handler.BatchHandler{
		Store:         st,
		KeysHandler:   keysH,
		AudioCacheDir: audioCacheDir,
		ProgressHub:   progressH,
	}
	pronunciationH := &handler.PronunciationHandler{Store: st}
	exportProfilesH := &handler.ExportProfilesHandler{Store: st}
	stitchH := &handler.StitchHandler{Store: st, AudioCacheDir: audioCacheDir}
	castH := &handler.CastHandler{Store: st, KeysHandler: keysH}
	stylesH := &handler.StylesHandler{Store: st}
	qcH := &handler.QcHandler{Store: st}
	clientH := &handler.ClientHandler{Store: st}
	providersH := &handler.ProvidersHandler{Store: st, KeysHandler: keysH}
	exportCacheDir := filepath.Join(filepath.Dir(audioCacheDir), "export_cache")
	exportsH := &handler.ExportsHandler{Store: st, AudioCacheDir: audioCacheDir, ExportCacheDir: exportCacheDir}
	scriptPrepH := &handler.ScriptPrepHandler{Store: st, KeysHandler: keysH}

	RegisterRoutes(mux, configH, keysH, historyH, voicesH, presetsH, favoritesH, cacheH, backupH, jobsH, projectsH, takesH, batchH, pronunciationH, exportProfilesH, stitchH, castH, stylesH, qcH, clientH, providersH, progressH, exportsH, scriptPrepH)

	if frontendFS != nil {
		fileServer := http.FileServer(http.FS(frontendFS))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.NotFound(w, r)
				return
			}

			path := r.URL.Path
			if path == "/" {
				path = "/index.html"
			}

			f, err := frontendFS.Open(strings.TrimPrefix(path, "/"))
			if err != nil {
				r.URL.Path = "/index.html"
			} else {
				_ = f.Close()
			}

			fileServer.ServeHTTP(w, r)
		})
	}

	return &Server{Mux: mux, Addr: addr}
}

// Handler returns the fully wrapped handler with middleware.
func (s *Server) Handler() http.Handler {
	var h http.Handler = s.Mux
	h = securityHeadersMiddleware(h)
	h = corsMiddleware(h)
	h = originProtectionMiddleware(h)
	h = rateLimitMiddleware(DefaultRateLimiterConfig())(h)
	h = loggingMiddleware(h)
	h = recoveryMiddleware(h)
	return h
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	return fmt.Errorf("server: %w", http.ListenAndServe(s.Addr, s.Handler()))
}
