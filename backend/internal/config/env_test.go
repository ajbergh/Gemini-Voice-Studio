// Copyright 2026 ajbergh
// SPDX-License-Identifier: Apache-2.0

package config

import "testing"

func TestDefaultConfigUsesLoopbackHost(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Host != "127.0.0.1" {
		t.Fatalf("expected loopback default host, got %q", cfg.Host)
	}
}

func TestApplyEnvironmentOverridesHost(t *testing.T) {
	t.Setenv("GVS_HOST", "0.0.0.0")

	cfg, err := ApplyEnvironment(DefaultConfig())
	if err != nil {
		t.Fatalf("apply environment: %v", err)
	}
	if cfg.Host != "0.0.0.0" {
		t.Fatalf("expected environment host override, got %q", cfg.Host)
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("validate environment configuration: %v", err)
	}
}

func TestValidateRejectsEmptyHost(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Host = ""
	if err := Validate(cfg); err == nil {
		t.Fatal("expected empty host validation error")
	}
}
