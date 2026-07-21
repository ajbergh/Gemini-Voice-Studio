// Copyright 2025 ajbergh
// SPDX-License-Identifier: Apache-2.0

// Package store provides the SQLite-backed persistence layer.
package store

import (
	"database/sql"
	"embed"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

const pendingRestoreSuffix = ".restore-pending"

// Store wraps a SQLite database connection.
type Store struct {
	db     *sql.DB
	dbPath string
}

// New applies any previously staged restore, opens SQLite, and prepares schema.
func New(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}
	if err := applyPendingRestore(dbPath); err != nil {
		return nil, fmt.Errorf("apply pending restore: %w", err)
	}
	db, err := openDatabase(dbPath)
	if err != nil {
		return nil, err
	}
	store := &Store{db: db, dbPath: dbPath}
	if err := store.prepareDatabase(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error { return s.db.Close() }

// DBPath returns the path to the SQLite database file.
func (s *Store) DBPath() string { return s.dbPath }

// DB returns the underlying sql.DB for handlers that need read-only catalogue queries.
func (s *Store) DB() *sql.DB { return s.db }

// Backup creates a consistent snapshot using VACUUM INTO.
func (s *Store) Backup(destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o700); err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}
	_ = os.Remove(destPath)
	if _, err := s.db.Exec("VACUUM INTO ?", destPath); err != nil {
		return fmt.Errorf("vacuum into: %w", err)
	}
	return nil
}

// Restore validates a candidate and stages it for atomic application at the
// next process start. This avoids replacing a live database underneath active
// requests and background render/export jobs.
func (s *Store) Restore(srcPath string) error {
	candidate, err := openDatabase(srcPath)
	if err != nil {
		return fmt.Errorf("open backup database: %w", err)
	}
	if err := prepareDatabase(candidate); err != nil {
		_ = candidate.Close()
		return fmt.Errorf("backup file is not compatible: %w", err)
	}
	if err := candidate.Close(); err != nil {
		return fmt.Errorf("close validated backup: %w", err)
	}

	pendingPath := s.dbPath + pendingRestoreSuffix
	if err := copyFileDurable(srcPath, pendingPath); err != nil {
		return fmt.Errorf("stage restored database: %w", err)
	}
	return nil
}

// RestorePending reports whether a staged database will be applied on restart.
func (s *Store) RestorePending() bool {
	_, err := os.Stat(s.dbPath + pendingRestoreSuffix)
	return err == nil
}

func applyPendingRestore(dbPath string) error {
	pendingPath := dbPath + pendingRestoreSuffix
	if _, err := os.Stat(pendingPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	backupPath := dbPath + ".pre-restore"
	_ = os.Remove(backupPath)
	if _, err := os.Stat(dbPath); err == nil {
		if err := os.Rename(dbPath, backupPath); err != nil {
			return fmt.Errorf("preserve current database: %w", err)
		}
	}
	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(dbPath + "-shm")
	if err := os.Rename(pendingPath, dbPath); err != nil {
		if _, backupErr := os.Stat(backupPath); backupErr == nil {
			_ = os.Rename(backupPath, dbPath)
		}
		return fmt.Errorf("publish restored database: %w", err)
	}
	_ = os.Chmod(dbPath, 0o600)
	return nil
}

func copyFileDurable(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".db-restore-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := io.Copy(temporary, input); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	_ = os.Remove(destination)
	return os.Rename(temporaryPath, destination)
}

// openDatabase uses one pooled connection so connection-scoped SQLite pragmas
// such as foreign_keys remain deterministic for all operations.
func openDatabase(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return db, nil
}

func prepareDatabase(db *sql.DB) error {
	if _, err := db.Exec(`
		PRAGMA journal_mode=WAL;
		PRAGMA foreign_keys=ON;
		PRAGMA synchronous=NORMAL;
		PRAGMA temp_store=MEMORY;
	`); err != nil {
		return fmt.Errorf("set pragmas: %w", err)
	}
	if err := migrateDB(db); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	addColumnIfMissing(db, "custom_presets", "color", "TEXT NOT NULL DEFAULT '#6366f1'")
	addColumnIfMissing(db, "custom_presets", "sort_order", "INTEGER NOT NULL DEFAULT 0")
	addColumnIfMissing(db, "segment_takes", "peak_dbfs", "REAL")
	addColumnIfMissing(db, "segment_takes", "rms_dbfs", "REAL")
	addColumnIfMissing(db, "segment_takes", "clipping_detected", "INTEGER NOT NULL DEFAULT 0")
	addColumnIfMissing(db, "segment_takes", "sample_rate", "INTEGER")
	addColumnIfMissing(db, "segment_takes", "channels", "INTEGER")
	addColumnIfMissing(db, "segment_takes", "format", "TEXT")
	addColumnIfMissing(db, "script_segments", "cast_profile_id", "INTEGER")
	addColumnIfMissing(db, "script_projects", "client_id", "INTEGER REFERENCES clients(id) ON DELETE SET NULL")
	addColumnIfMissing(db, "pronunciation_dictionaries", "client_id", "INTEGER REFERENCES clients(id) ON DELETE SET NULL")
	addColumnIfMissing(db, "script_projects", "fallback_provider", "TEXT")
	addColumnIfMissing(db, "script_projects", "fallback_model", "TEXT")
	addColumnIfMissing(db, "script_segments", "fallback_provider", "TEXT")
	addColumnIfMissing(db, "script_segments", "fallback_model", "TEXT")
	addColumnIfMissing(db, "clients", "fallback_provider", "TEXT")
	addColumnIfMissing(db, "clients", "fallback_model", "TEXT")
	addColumnIfMissing(db, "segment_takes", "provider_voice", "TEXT")
	addColumnIfMissing(db, "segment_takes", "app_voice_name", "TEXT")
	addColumnIfMissing(db, "segment_takes", "preset_id", "INTEGER")
	addColumnIfMissing(db, "segment_takes", "style_id", "INTEGER")
	addColumnIfMissing(db, "segment_takes", "accent_id", "TEXT")
	addColumnIfMissing(db, "segment_takes", "cast_profile_id", "INTEGER")
	addColumnIfMissing(db, "segment_takes", "dictionary_hash", "TEXT")
	addColumnIfMissing(db, "segment_takes", "prompt_hash", "TEXT")
	addColumnIfMissing(db, "segment_takes", "settings_json", "TEXT")

	if err := validateSchema(db); err != nil {
		return fmt.Errorf("validate schema: %w", err)
	}
	return nil
}

func (s *Store) prepareDatabase() error { return prepareDatabase(s.db) }

// migrateDB executes each embedded migration exactly once and records it in a
// schema_migrations ledger. Existing databases safely bootstrap because the
// historical migration scripts are idempotent.
func migrateDB(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		name TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}
	rows, err := db.Query("SELECT name FROM schema_migrations")
	if err != nil {
		return fmt.Errorf("list applied migrations: %w", err)
	}
	applied := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return err
		}
		applied[name] = true
	}
	if err := rows.Close(); err != nil {
		return err
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".sql") || applied[name] {
			continue
		}
		data, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", name, err)
		}
		if _, err := tx.Exec(string(data)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("execute migration %s: %w", name, err)
		}
		if _, err := tx.Exec("INSERT INTO schema_migrations (name, applied_at) VALUES (?, ?)", name, time.Now().UTC().Format(time.RFC3339)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
		slog.Info("applied migration", "file", name)
	}
	return nil
}

// SchemaVersion returns the newest recorded migration filename.
func (s *Store) SchemaVersion() string {
	var version string
	_ = s.db.QueryRow("SELECT COALESCE(MAX(name), '') FROM schema_migrations").Scan(&version)
	return version
}

func addColumnIfMissing(db *sql.DB, table, column, definition string) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		slog.Warn("failed to inspect table", "table", table, "error", err)
		return
	}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue *string
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err == nil && strings.EqualFold(name, column) {
			_ = rows.Close()
			return
		}
	}
	_ = rows.Close()
	statement := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition)
	if _, err := db.Exec(statement); err != nil {
		slog.Warn("failed to add compatibility column", "table", table, "column", column, "error", err)
	} else {
		slog.Info("added compatibility column", "table", table, "column", column)
	}
}

func validateSchema(db *sql.DB) error {
	required := map[string][]string{
		"api_keys":                          {"id", "provider", "encrypted", "nonce"},
		"config":                            {"key", "value"},
		"history":                           {"id", "type", "voice_name", "input_text", "result_json", "audio_path", "created_at"},
		"voices":                            {"name", "pitch", "gender", "characteristics", "audio_sample_url", "file_uri", "analysis_json", "image_url"},
		"custom_presets":                    {"id", "name", "voice_name", "system_instruction", "sample_text", "audio_path", "source_query", "metadata_json", "color", "sort_order", "created_at", "updated_at"},
		"favorites":                         {"voice_name", "created_at"},
		"preset_tags":                       {"id", "preset_id", "tag", "color"},
		"preset_versions":                   {"id", "preset_id", "name", "voice_name", "system_instruction", "sample_text", "color", "metadata_json", "created_at"},
		"api_key_pool":                      {"id", "provider", "label", "encrypted", "nonce", "is_active", "error_count", "last_used_at", "created_at", "updated_at"},
		"jobs":                              {"id", "job_type", "status", "project_id", "section_id", "segment_id", "total_items", "completed_items", "failed_items", "percent", "message", "error", "error_code", "metadata_json", "created_at", "updated_at", "completed_at"},
		"job_items":                         {"id", "job_id", "segment_id", "status", "attempt_count", "last_error", "sort_order", "created_at", "updated_at"},
		"script_projects":                   {"id", "title", "kind", "description", "status", "default_voice_name", "default_preset_id", "default_style_id", "default_accent_id", "default_language_code", "default_provider", "default_model", "fallback_provider", "fallback_model", "client_id", "metadata_json", "created_at", "updated_at"},
		"script_sections":                   {"id", "project_id", "parent_id", "kind", "title", "sort_order", "metadata_json", "created_at", "updated_at"},
		"script_segments":                   {"id", "project_id", "section_id", "title", "script_text", "speaker_label", "voice_name", "cast_profile_id", "preset_id", "style_id", "accent_id", "language_code", "provider", "model", "fallback_provider", "fallback_model", "status", "content_hash", "sort_order", "metadata_json", "created_at", "updated_at"},
		"segment_takes":                     {"id", "project_id", "segment_id", "take_number", "voice_name", "speaker_label", "language_code", "provider", "model", "provider_voice", "app_voice_name", "preset_id", "style_id", "accent_id", "cast_profile_id", "dictionary_hash", "prompt_hash", "settings_json", "system_instruction", "script_text", "audio_path", "duration_seconds", "peak_dbfs", "rms_dbfs", "clipping_detected", "sample_rate", "channels", "format", "content_hash", "status", "metadata_json", "created_at"},
		"take_notes":                        {"id", "take_id", "note", "created_at"},
		"pronunciation_dictionaries":        {"id", "project_id", "name", "created_at", "updated_at"},
		"pronunciation_entries":             {"id", "dictionary_id", "raw_word", "replacement", "is_regex", "enabled", "sort_order", "created_at", "updated_at"},
		"global_pronunciation_dictionaries": {"id", "name", "created_at", "updated_at"},
		"global_pronunciation_entries":      {"id", "dictionary_id", "raw_word", "replacement", "is_regex", "enabled", "sort_order", "created_at", "updated_at"},
		"export_profiles":                   {"id", "name", "target_kind", "trim_silence", "silence_threshold_db", "leading_silence_ms", "trailing_silence_ms", "inter_segment_silence_ms", "normalize_peak_db", "is_builtin", "metadata_json", "created_at", "updated_at"},
		"cast_profiles":                     {"id", "project_id", "series_id", "name", "role", "description", "voice_name", "preset_id", "style_id", "accent_id", "language_code", "age_impression", "emotional_range", "sample_lines_json", "pronunciation_notes", "metadata_json", "sort_order", "created_at", "updated_at"},
		"cast_profile_versions":             {"id", "profile_id", "name", "role", "description", "voice_name", "preset_id", "style_id", "accent_id", "language_code", "age_impression", "emotional_range", "sample_lines_json", "pronunciation_notes", "metadata_json", "sort_order", "created_at"},
		"provider_voice_mappings":           {"id", "project_id", "source_provider", "source_voice", "target_provider", "target_voice", "notes", "created_at", "updated_at"},
		"export_jobs":                       {"id", "project_id", "export_profile_id", "status", "output_path", "error", "metadata_json", "created_at", "updated_at"},
		"export_job_items":                  {"id", "export_job_id", "asset_type", "asset_id", "output_name", "status", "error"},
		"script_prep_jobs":                  {"id", "project_id", "raw_script_hash", "raw_script", "result_json", "status", "error", "created_at", "updated_at"},
	}
	for table, columns := range required {
		available, err := tableColumns(db, table)
		if err != nil {
			return err
		}
		for _, column := range columns {
			if _, ok := available[column]; !ok {
				return fmt.Errorf("missing required column %s.%s", table, column)
			}
		}
	}
	return nil
}

func tableColumns(db *sql.DB, table string) (map[string]struct{}, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, fmt.Errorf("inspect table %s: %w", table, err)
	}
	defer rows.Close()
	columns := map[string]struct{}{}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue *string
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, fmt.Errorf("scan table info for %s: %w", table, err)
		}
		columns[strings.ToLower(name)] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate table info for %s: %w", table, err)
	}
	if len(columns) == 0 {
		return nil, fmt.Errorf("missing required table %s", table)
	}
	return columns, nil
}
