package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvFileSetsMissingValues(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("SPOTTER_DOCKER", "")
	if err := os.Unsetenv("OPENAI_API_KEY"); err != nil {
		t.Fatalf("unset OPENAI_API_KEY: %v", err)
	}
	if err := os.Unsetenv("SPOTTER_DOCKER"); err != nil {
		t.Fatalf("unset SPOTTER_DOCKER: %v", err)
	}

	path := filepath.Join(t.TempDir(), ".env")
	raw := []byte("# local env\nOPENAI_API_KEY=\"ollama\"\nSPOTTER_DOCKER=1\n")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	if err := LoadEnvFile(path); err != nil {
		t.Fatalf("LoadEnvFile() error = %v", err)
	}

	if got := os.Getenv("OPENAI_API_KEY"); got != "ollama" {
		t.Fatalf("OPENAI_API_KEY = %q, want %q", got, "ollama")
	}
	if got := os.Getenv("SPOTTER_DOCKER"); got != "1" {
		t.Fatalf("SPOTTER_DOCKER = %q, want %q", got, "1")
	}
}

func TestLoadEnvFileDoesNotOverrideExistingValues(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "real-key")

	path := filepath.Join(t.TempDir(), ".env")
	raw := []byte("OPENAI_API_KEY=ollama\n")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	if err := LoadEnvFile(path); err != nil {
		t.Fatalf("LoadEnvFile() error = %v", err)
	}

	if got := os.Getenv("OPENAI_API_KEY"); got != "real-key" {
		t.Fatalf("OPENAI_API_KEY = %q, want existing value %q", got, "real-key")
	}
}

func TestLoadEnvFileAllowsMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")

	if err := LoadEnvFile(path); err != nil {
		t.Fatalf("LoadEnvFile() error = %v, want nil for missing file", err)
	}
}
