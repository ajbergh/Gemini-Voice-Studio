// Copyright 2026 ajbergh
// SPDX-License-Identifier: Apache-2.0

package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"io"
	"testing"
)

func TestVersionedEncryptionRoundTrip(t *testing.T) {
	bundle, err := DeriveKey("correct horse battery staple", t.TempDir())
	if err != nil {
		t.Fatalf("derive key: %v", err)
	}
	ciphertext, nonce, err := Encrypt(bundle, []byte("secret-api-key"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	plaintext, err := Decrypt(bundle, ciphertext, nonce)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(plaintext) != "secret-api-key" {
		t.Fatalf("plaintext = %q", plaintext)
	}
}

func TestLegacyCiphertextRemainsReadable(t *testing.T) {
	passphrase := "legacy-passphrase"
	bundle, err := DeriveKey(passphrase, t.TempDir())
	if err != nil {
		t.Fatalf("derive key: %v", err)
	}
	legacy := sha256.Sum256([]byte(passphrase))
	block, err := aes.NewCipher(legacy[:])
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("new gcm: %v", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		t.Fatalf("nonce: %v", err)
	}
	legacyCiphertext := gcm.Seal(nil, nonce, []byte("legacy-key"), nil)
	plaintext, err := Decrypt(bundle, legacyCiphertext, nonce)
	if err != nil {
		t.Fatalf("decrypt legacy: %v", err)
	}
	if string(plaintext) != "legacy-key" {
		t.Fatalf("plaintext = %q", plaintext)
	}
}
