// Copyright 2025 ajbergh
// SPDX-License-Identifier: Apache-2.0

// Package crypto provides versioned AES-256-GCM encryption for API keys.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
)

const (
	kdfIterations = 210000
	saltSize      = 16
)

var currentCiphertextPrefix = []byte("GVS2")

// DeriveKey returns a versioned key bundle. The first 32 bytes are derived with
// PBKDF2-HMAC-SHA256 and a random per-installation salt. The final 32 bytes are
// the legacy SHA-256 key so existing encrypted rows remain readable and are
// transparently migrated whenever they are rewritten.
func DeriveKey(passphrase, dataDir string) ([]byte, error) {
	secret := passphrase
	if secret == "" {
		machineSecret, err := machineID()
		if err != nil {
			return nil, fmt.Errorf("derive machine identity: %w", err)
		}
		secret = machineSecret
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create encryption data directory: %w", err)
	}
	salt, err := loadOrCreateSalt(filepath.Join(dataDir, "encryption.salt"))
	if err != nil {
		return nil, err
	}
	current := pbkdf2SHA256([]byte(secret), salt, kdfIterations, 32)
	legacyDigest := sha256.Sum256([]byte(secret))
	bundle := make([]byte, 0, 64)
	bundle = append(bundle, current...)
	bundle = append(bundle, legacyDigest[:]...)
	return bundle, nil
}

// Encrypt encrypts plaintext with the current versioned key and prefixes the
// ciphertext so Decrypt can distinguish it from legacy rows.
func Encrypt(keyBundle, plaintext []byte) (ciphertext, nonce []byte, err error) {
	key, err := currentKey(keyBundle)
	if err != nil {
		return nil, nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("new gcm: %w", err)
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("generate nonce: %w", err)
	}
	sealed := gcm.Seal(nil, nonce, plaintext, nil)
	ciphertext = append(append([]byte(nil), currentCiphertextPrefix...), sealed...)
	return ciphertext, nonce, nil
}

// Decrypt supports both current prefixed ciphertext and legacy unprefixed rows.
func Decrypt(keyBundle, ciphertext, nonce []byte) ([]byte, error) {
	useCurrent := len(ciphertext) >= len(currentCiphertextPrefix) && hmac.Equal(ciphertext[:len(currentCiphertextPrefix)], currentCiphertextPrefix)
	key, err := legacyKey(keyBundle)
	if useCurrent {
		key, err = currentKey(keyBundle)
		ciphertext = ciphertext[len(currentCiphertextPrefix):]
	}
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, errors.New("invalid nonce size")
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}

func currentKey(bundle []byte) ([]byte, error) {
	if len(bundle) >= 64 {
		return bundle[:32], nil
	}
	if len(bundle) == 32 {
		return bundle, nil
	}
	return nil, fmt.Errorf("invalid encryption key bundle length %d", len(bundle))
}

func legacyKey(bundle []byte) ([]byte, error) {
	if len(bundle) >= 64 {
		return bundle[32:64], nil
	}
	if len(bundle) == 32 {
		return bundle, nil
	}
	return nil, fmt.Errorf("invalid encryption key bundle length %d", len(bundle))
}

func loadOrCreateSalt(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		if len(data) != saltSize {
			return nil, fmt.Errorf("invalid encryption salt length %d", len(data))
		}
		return data, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read encryption salt: %w", err)
	}
	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("generate encryption salt: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return loadOrCreateSalt(path)
		}
		return nil, fmt.Errorf("create encryption salt: %w", err)
	}
	if _, err := file.Write(salt); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("write encryption salt: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("sync encryption salt: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close encryption salt: %w", err)
	}
	return salt, nil
}

func pbkdf2SHA256(password, salt []byte, iterations, keyLength int) []byte {
	hashLength := sha256.Size
	blocks := (keyLength + hashLength - 1) / hashLength
	derived := make([]byte, 0, blocks*hashLength)
	for blockIndex := 1; blockIndex <= blocks; blockIndex++ {
		mac := hmac.New(sha256.New, password)
		_, _ = mac.Write(salt)
		_, _ = mac.Write([]byte{byte(blockIndex >> 24), byte(blockIndex >> 16), byte(blockIndex >> 8), byte(blockIndex)})
		u := mac.Sum(nil)
		result := append([]byte(nil), u...)
		for iteration := 1; iteration < iterations; iteration++ {
			mac = hmac.New(sha256.New, password)
			_, _ = mac.Write(u)
			u = mac.Sum(nil)
			for index := range result {
				result[index] ^= u[index]
			}
		}
		derived = append(derived, result...)
	}
	return derived[:keyLength]
}

func machineID() (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown-host"
	}
	currentUser, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("get current user: %w", err)
	}
	return fmt.Sprintf("gemini-voice-studio:%s:%s", hostname, currentUser.Username), nil
}
