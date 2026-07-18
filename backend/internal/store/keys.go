// Copyright 2025 ajbergh
// SPDX-License-Identifier: Apache-2.0

// Package store — keys.go implements encrypted API key and pooled-key health
// persistence. Plaintext credentials never enter this layer.
package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// APIKeyRow represents a stored encrypted API key.
type APIKeyRow struct {
	ID        int64  `json:"id"`
	Provider  string `json:"provider"`
	Encrypted []byte `json:"-"`
	Nonce     []byte `json:"-"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func (s *Store) ListAPIKeyProviders() ([]APIKeyRow, error) {
	rows, err := s.db.Query("SELECT id, provider, created_at, updated_at FROM api_keys ORDER BY provider")
	if err != nil {
		return nil, fmt.Errorf("query api_keys: %w", err)
	}
	defer rows.Close()
	keys := make([]APIKeyRow, 0)
	for rows.Next() {
		var key APIKeyRow
		if err := rows.Scan(&key.ID, &key.Provider, &key.CreatedAt, &key.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan api_key: %w", err)
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (s *Store) GetAPIKey(provider string) (*APIKeyRow, error) {
	var key APIKeyRow
	err := s.db.QueryRow(
		"SELECT id, provider, encrypted, nonce, created_at, updated_at FROM api_keys WHERE provider = ?",
		provider,
	).Scan(&key.ID, &key.Provider, &key.Encrypted, &key.Nonce, &key.CreatedAt, &key.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("query api_key %s: %w", provider, err)
	}
	return &key, nil
}

func (s *Store) UpsertAPIKey(provider string, encrypted, nonce []byte) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`INSERT INTO api_keys (provider, encrypted, nonce, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(provider) DO UPDATE SET encrypted = excluded.encrypted, nonce = excluded.nonce, updated_at = excluded.updated_at`,
		provider, encrypted, nonce, now, now,
	)
	if err != nil {
		return fmt.Errorf("upsert api_key: %w", err)
	}
	return nil
}

func (s *Store) DeleteAPIKey(provider string) error {
	result, err := s.db.Exec("DELETE FROM api_keys WHERE provider = ?", provider)
	if err != nil {
		return fmt.Errorf("delete api_key: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return fmt.Errorf("api key for provider %q not found", provider)
	}
	return nil
}

// APIKeyPoolRow represents a key and its provider-health state.
type APIKeyPoolRow struct {
	ID            int64  `json:"id"`
	Provider      string `json:"provider"`
	Label         string `json:"label"`
	Encrypted     []byte `json:"-"`
	Nonce         []byte `json:"-"`
	IsActive      bool   `json:"is_active"`
	ErrorCount    int    `json:"error_count"`
	LastUsedAt    string `json:"last_used_at,omitempty"`
	CooldownUntil string `json:"cooldown_until,omitempty"`
	LastError     string `json:"last_error,omitempty"`
	LastStatus    string `json:"last_status,omitempty"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

const poolKeyColumns = `id, provider, label, encrypted, nonce, is_active, error_count,
	COALESCE(last_used_at,''), COALESCE(cooldown_until,''), COALESCE(last_error,''),
	COALESCE(last_status,''), created_at, updated_at`

func scanPoolKey(scanner interface{ Scan(...any) error }) (APIKeyPoolRow, error) {
	var key APIKeyPoolRow
	err := scanner.Scan(
		&key.ID, &key.Provider, &key.Label, &key.Encrypted, &key.Nonce,
		&key.IsActive, &key.ErrorCount, &key.LastUsedAt, &key.CooldownUntil,
		&key.LastError, &key.LastStatus, &key.CreatedAt, &key.UpdatedAt,
	)
	return key, err
}

func (s *Store) ListAPIKeyPool(provider string) ([]APIKeyPoolRow, error) {
	rows, err := s.db.Query(
		`SELECT `+poolKeyColumns+` FROM api_key_pool WHERE provider = ? ORDER BY id`,
		provider,
	)
	if err != nil {
		return nil, fmt.Errorf("query api_key_pool: %w", err)
	}
	defer rows.Close()
	keys := make([]APIKeyPoolRow, 0)
	for rows.Next() {
		key, err := scanPoolKey(rows)
		if err != nil {
			return nil, fmt.Errorf("scan api_key_pool: %w", err)
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (s *Store) AddAPIKeyToPool(provider, label string, encrypted, nonce []byte) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.Exec(
		`INSERT INTO api_key_pool (provider, label, encrypted, nonce, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		provider, strings.TrimSpace(label), encrypted, nonce, now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("insert api_key_pool: %w", err)
	}
	return result.LastInsertId()
}

func (s *Store) DeleteAPIKeyFromPool(id int64) error {
	result, err := s.db.Exec("DELETE FROM api_key_pool WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete api_key_pool: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return fmt.Errorf("pool key %d not found", id)
	}
	return nil
}

// GetNextPoolKey atomically leases the least-recently-used healthy key. A
// transaction prevents simultaneous callers from selecting the same row.
func (s *Store) GetNextPoolKey(provider string) (*APIKeyPoolRow, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin key lease: %w", err)
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	key, err := scanPoolKey(tx.QueryRow(
		`SELECT `+poolKeyColumns+`
		 FROM api_key_pool
		 WHERE provider = ? AND is_active = 1
		   AND (cooldown_until IS NULL OR cooldown_until = '' OR cooldown_until <= ?)
		 ORDER BY CASE WHEN last_used_at IS NULL OR last_used_at = '' THEN 0 ELSE 1 END,
		          last_used_at ASC, id ASC
		 LIMIT 1`,
		provider, now,
	))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("select pooled key: %w", err)
	}
	if _, err := tx.Exec(
		"UPDATE api_key_pool SET last_used_at = ?, last_status = 'leased', updated_at = ? WHERE id = ?",
		now, now, key.ID,
	); err != nil {
		return nil, fmt.Errorf("claim pooled key: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit key lease: %w", err)
	}
	key.LastUsedAt = now
	key.LastStatus = "leased"
	return &key, nil
}

// ReportPoolKeySuccess clears transient health state after a successful call.
func (s *Store) ReportPoolKeySuccess(id int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`UPDATE api_key_pool
		 SET error_count = 0, cooldown_until = NULL, last_error = NULL,
		     last_status = 'healthy', is_active = 1, updated_at = ?
		 WHERE id = ?`,
		now, id,
	)
	return err
}

// ReportPoolKeyFailure records provider-aware health state. Authentication
// failures deactivate immediately; quota/rate failures enter a cooldown; other
// failures deactivate only after repeated errors.
func (s *Store) ReportPoolKeyFailure(id int64, status, message string, cooldown time.Duration) error {
	nowTime := time.Now().UTC()
	now := nowTime.Format(time.RFC3339)
	cooldownUntil := ""
	if cooldown > 0 {
		cooldownUntil = nowTime.Add(cooldown).Format(time.RFC3339)
	}
	status = strings.ToLower(strings.TrimSpace(status))
	deactivateImmediately := status == "unauthorized" || status == "forbidden" || status == "invalid_key"
	_, err := s.db.Exec(
		`UPDATE api_key_pool
		 SET error_count = error_count + 1,
		     is_active = CASE WHEN ? = 1 OR error_count + 1 >= 5 THEN 0 ELSE 1 END,
		     cooldown_until = NULLIF(?, ''), last_error = ?, last_status = ?, updated_at = ?
		 WHERE id = ?`,
		boolToInt(deactivateImmediately), cooldownUntil, truncateKeyError(message), status, now, id,
	)
	return err
}

// MarkPoolKeyError preserves the old API for local decryption failures.
func (s *Store) MarkPoolKeyError(id int64) error {
	return s.ReportPoolKeyFailure(id, "decrypt_error", "failed to decrypt pooled key", 0)
}

func (s *Store) ResetPoolKeyErrors(id int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.Exec(
		`UPDATE api_key_pool SET error_count = 0, is_active = 1,
		 cooldown_until = NULL, last_error = NULL, last_status = 'reset', updated_at = ? WHERE id = ?`,
		now, id,
	)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return fmt.Errorf("pool key %d not found", id)
	}
	return nil
}

func truncateKeyError(message string) string {
	message = strings.TrimSpace(message)
	if len(message) > 500 {
		return message[:497] + "..."
	}
	return message
}
