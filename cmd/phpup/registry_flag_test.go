package main

import "testing"

func TestResolveRegistry_FlagBeatsEnv(t *testing.T) {
	t.Setenv("INPUT_REGISTRY", "env-input")
	t.Setenv("PHPUP_REGISTRY", "env-phpup")
	got := resolveRegistry("flag-value")
	if got != "flag-value" {
		t.Errorf("resolveRegistry flag = %q, want flag-value", got)
	}
}

func TestResolveRegistry_InputEnvBeatsPhpupEnv(t *testing.T) {
	t.Setenv("INPUT_REGISTRY", "env-input")
	t.Setenv("PHPUP_REGISTRY", "env-phpup")
	got := resolveRegistry("")
	if got != "env-input" {
		t.Errorf("resolveRegistry = %q, want env-input", got)
	}
}

func TestResolveRegistry_PhpupEnvUsedWhenInputEmpty(t *testing.T) {
	t.Setenv("INPUT_REGISTRY", "")
	t.Setenv("PHPUP_REGISTRY", "env-phpup")
	got := resolveRegistry("")
	if got != "env-phpup" {
		t.Errorf("resolveRegistry = %q, want env-phpup", got)
	}
}

func TestResolveRegistry_DefaultsToGHCR(t *testing.T) {
	t.Setenv("INPUT_REGISTRY", "")
	t.Setenv("PHPUP_REGISTRY", "")
	got := resolveRegistry("")
	if got != "ghcr.io/buildrush" {
		t.Errorf("resolveRegistry = %q, want ghcr.io/buildrush", got)
	}
}
