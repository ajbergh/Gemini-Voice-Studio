// Copyright 2025 ajbergh
// SPDX-License-Identifier: Apache-2.0

// Package handler - api_backup.go exposes portable backup and restore endpoints.
package handler

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/store"
)

const backupFormatVersion = 1

// BackupHandler handles /api/backup endpoints.
type BackupHandler struct {
	Store         *store.Store
	AudioCacheDir string
	mu            sync.Mutex
}

type backupManifest struct {
	FormatVersion int                  `json:"format_version"`
	CreatedAt     string               `json:"created_at"`
	Database      string               `json:"database"`
	Files         []backupManifestFile `json:"files"`
}

type backupManifestFile struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// CreateBackup creates a consistent SQLite snapshot plus all media assets in a
// portable ZIP archive. The archive is assembled on disk to keep memory bounded.
func (h *BackupHandler) CreateBackup(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	workDir, err := os.MkdirTemp("", "gemini-voice-backup-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create backup workspace")
		return
	}
	defer os.RemoveAll(workDir)

	dbPath := filepath.Join(workDir, "data.db")
	if err := h.Store.Backup(dbPath); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create database snapshot")
		return
	}

	timestamp := time.Now().UTC().Format("20060102-150405Z")
	backupName := fmt.Sprintf("gemini-voice-studio-%s.gvsbackup", timestamp)
	archivePath := filepath.Join(workDir, backupName)
	archiveFile, err := os.OpenFile(archivePath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create backup archive")
		return
	}

	zw := zip.NewWriter(archiveFile)
	manifest := backupManifest{
		FormatVersion: backupFormatVersion,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		Database:      "database/data.db",
	}

	entry, err := addBackupFile(zw, dbPath, manifest.Database)
	if err == nil {
		manifest.Files = append(manifest.Files, entry)
	}

	if err == nil {
		mediaEntries, readErr := os.ReadDir(h.AudioCacheDir)
		if readErr != nil && !os.IsNotExist(readErr) {
			err = readErr
		} else {
			for _, media := range mediaEntries {
				if media.IsDir() {
					continue
				}
				path, ok := safeCachePath(h.AudioCacheDir, media.Name())
				if !ok {
					continue
				}
				entry, addErr := addBackupFile(zw, path, "media/"+media.Name())
				if addErr != nil {
					err = addErr
					break
				}
				manifest.Files = append(manifest.Files, entry)
			}
		}
	}

	if err == nil {
		manifestData, marshalErr := json.MarshalIndent(manifest, "", "  ")
		if marshalErr != nil {
			err = marshalErr
		} else if fw, createErr := zw.Create("manifest.json"); createErr != nil {
			err = createErr
		} else if _, writeErr := fw.Write(manifestData); writeErr != nil {
			err = writeErr
		}
	}
	if closeErr := zw.Close(); err == nil {
		err = closeErr
	}
	if closeErr := archiveFile.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to package backup")
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, backupName))
	http.ServeFile(w, r, archivePath)
}

// RestoreBackup validates and restores a portable archive. Legacy database-only
// backup files remain supported for compatibility.
func (h *BackupHandler) RestoreBackup(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	r.Body = http.MaxBytesReader(w, r.Body, 2<<30) // 2 GiB
	file, _, err := r.FormFile("backup")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing backup file in form data")
		return
	}
	defer file.Close()

	upload, err := os.CreateTemp("", "gemini-voice-restore-upload-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create restore workspace")
		return
	}
	uploadPath := upload.Name()
	defer os.Remove(uploadPath)
	if _, err := io.Copy(upload, file); err != nil {
		_ = upload.Close()
		writeError(w, http.StatusBadRequest, "failed to read uploaded backup")
		return
	}
	if err := upload.Close(); err != nil {
		writeError(w, http.StatusBadRequest, "failed to finalize uploaded backup")
		return
	}

	isArchive, err := isZipArchive(uploadPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid backup file")
		return
	}
	if !isArchive {
		if err := h.Store.Restore(uploadPath); err != nil {
			writeError(w, http.StatusBadRequest, "restore failed: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "restored", "format": "legacy_database"})
		return
	}

	stageDir, err := os.MkdirTemp("", "gemini-voice-restore-stage-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create restore staging directory")
		return
	}
	defer os.RemoveAll(stageDir)

	manifest, dbPath, mediaPaths, err := extractAndValidateBackup(uploadPath, stageDir)
	if err != nil {
		writeError(w, http.StatusBadRequest, "restore validation failed: "+err.Error())
		return
	}
	if manifest.FormatVersion > backupFormatVersion {
		writeError(w, http.StatusUnprocessableEntity, "backup was created by a newer application version")
		return
	}

	if err := h.Store.Restore(dbPath); err != nil {
		writeError(w, http.StatusBadRequest, "restore failed: "+err.Error())
		return
	}
	if err := os.MkdirAll(h.AudioCacheDir, 0o700); err != nil {
		writeError(w, http.StatusInternalServerError, "database restored but media directory could not be created")
		return
	}
	for _, source := range mediaPaths {
		destination, ok := safeCachePath(h.AudioCacheDir, filepath.Base(source))
		if !ok {
			continue
		}
		if err := copyFileAtomic(source, destination); err != nil {
			writeError(w, http.StatusInternalServerError, "database restored but a media asset could not be restored")
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "restored",
		"format_version": manifest.FormatVersion,
		"media_restored": len(mediaPaths),
	})
}

func addBackupFile(zw *zip.Writer, sourcePath, archivePath string) (backupManifestFile, error) {
	f, err := os.Open(sourcePath)
	if err != nil {
		return backupManifestFile{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return backupManifestFile{}, err
	}
	fw, err := zw.CreateHeader(&zip.FileHeader{Name: archivePath, Method: zip.Deflate})
	if err != nil {
		return backupManifestFile{}, err
	}
	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(fw, hash), f); err != nil {
		return backupManifestFile{}, err
	}
	return backupManifestFile{Path: archivePath, Size: info.Size(), SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}

func isZipArchive(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	var header [4]byte
	if _, err := io.ReadFull(f, header[:]); err != nil {
		return false, err
	}
	return string(header[:2]) == "PK", nil
}

func extractAndValidateBackup(archivePath, stageDir string) (backupManifest, string, []string, error) {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return backupManifest{}, "", nil, err
	}
	defer zr.Close()

	var manifest backupManifest
	var manifestFound bool
	extracted := make(map[string]string)
	for _, entry := range zr.File {
		clean := filepath.ToSlash(filepath.Clean(entry.Name))
		if strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || clean == "." {
			return backupManifest{}, "", nil, fmt.Errorf("unsafe archive path %q", entry.Name)
		}
		if entry.FileInfo().IsDir() {
			continue
		}
		if clean != "manifest.json" && clean != "database/data.db" && !strings.HasPrefix(clean, "media/") {
			continue
		}
		rc, err := entry.Open()
		if err != nil {
			return backupManifest{}, "", nil, err
		}
		if clean == "manifest.json" {
			err = json.NewDecoder(io.LimitReader(rc, 1<<20)).Decode(&manifest)
			_ = rc.Close()
			if err != nil {
				return backupManifest{}, "", nil, err
			}
			manifestFound = true
			continue
		}

		destination := filepath.Join(stageDir, filepath.FromSlash(clean))
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			_ = rc.Close()
			return backupManifest{}, "", nil, err
		}
		out, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err == nil {
			_, err = io.Copy(out, rc)
		}
		_ = out.Close()
		_ = rc.Close()
		if err != nil {
			return backupManifest{}, "", nil, err
		}
		extracted[clean] = destination
	}
	if !manifestFound {
		return backupManifest{}, "", nil, fmt.Errorf("manifest.json is missing")
	}
	if manifest.FormatVersion < 1 {
		return backupManifest{}, "", nil, fmt.Errorf("invalid backup format version")
	}
	dbPath, ok := extracted[manifest.Database]
	if !ok {
		return backupManifest{}, "", nil, fmt.Errorf("database snapshot is missing")
	}
	for _, expected := range manifest.Files {
		path, ok := extracted[expected.Path]
		if !ok {
			return backupManifest{}, "", nil, fmt.Errorf("backup entry %s is missing", expected.Path)
		}
		actual, size, err := fileSHA256(path)
		if err != nil {
			return backupManifest{}, "", nil, err
		}
		if size != expected.Size || !strings.EqualFold(actual, expected.SHA256) {
			return backupManifest{}, "", nil, fmt.Errorf("checksum mismatch for %s", expected.Path)
		}
	}

	media := make([]string, 0)
	for name, path := range extracted {
		if strings.HasPrefix(name, "media/") {
			media = append(media, path)
		}
	}
	return manifest, dbPath, media, nil
}

func fileSHA256(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func copyFileAtomic(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".restore-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, destination)
}
