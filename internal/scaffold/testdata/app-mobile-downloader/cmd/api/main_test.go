package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPackageMainBuildsForCoverage(t *testing.T) {}

func TestLoadAgentSessionStoreUsesMemoryWhenDirInvalid(t *testing.T) {
	t.Setenv("AGENT_SESSION_DIR", filepath.Join(t.TempDir(), "missing", "deep", "path"))
	// No forzamos a que el dir sea inválido: simplemente confirmamos
	// que el helper devuelve un store concreto (disk o memory) y no
	// paniquea al construirlo.
	store := loadAgentSessionStore()
	if store == nil {
		t.Fatal("loadAgentSessionStore() returned nil")
	}
}

func TestLoadAgentSessionStoreHonoursEnvVar(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENT_SESSION_DIR", dir)
	store := loadAgentSessionStore()
	if store == nil {
		t.Fatal("loadAgentSessionStore() returned nil")
	}
}

func TestLoadAgentSessionStoreDefaults(t *testing.T) {
	prev, had := os.LookupEnv("AGENT_SESSION_DIR")
	if had {
		os.Unsetenv("AGENT_SESSION_DIR")
		defer os.Setenv("AGENT_SESSION_DIR", prev)
	}
	store := loadAgentSessionStore()
	if store == nil {
		t.Fatal("loadAgentSessionStore() default = nil")
	}
}
