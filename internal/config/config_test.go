package config

import "testing"

func TestDefaultRefreshIntervalIsTwoHours(t *testing.T) {
	cfg := Default()

	if cfg.Refresh.IntervalSeconds != 7200 {
		t.Fatalf("refresh interval = %d, want 7200", cfg.Refresh.IntervalSeconds)
	}
}

func TestDefaultStorageAuditFile(t *testing.T) {
	cfg := Default()

	if cfg.Storage.AuditFile != "./data/refresh_log.jsonl" {
		t.Fatalf("audit file = %q, want %q", cfg.Storage.AuditFile, "./data/refresh_log.jsonl")
	}
}

func TestLoadParsesOpenAIBaseURL(t *testing.T) {
	t.Setenv("HOME", "")
	raw := []byte(`openai:
  enabled: true
  base_url: "http://localhost:11434/v1"
  model: "qwen3:4b"
  timeout_seconds: 60
`)

	cfg := Default()
	if err := parseYAML(raw, &cfg); err != nil {
		t.Fatalf("parseYAML() error = %v", err)
	}
	cfg.applyDefaults()

	if cfg.OpenAI.BaseURL != "http://localhost:11434/v1" {
		t.Fatalf("OpenAI.BaseURL = %q, want %q", cfg.OpenAI.BaseURL, "http://localhost:11434/v1")
	}
}

func TestLoadParsesStorageAuditFile(t *testing.T) {
	raw := []byte(`storage:
  file: "./data/state.json"
  audit_file: "./data/custom_refresh_log.jsonl"
`)

	cfg := Default()
	if err := parseYAML(raw, &cfg); err != nil {
		t.Fatalf("parseYAML() error = %v", err)
	}
	cfg.applyDefaults()

	if cfg.Storage.AuditFile != "./data/custom_refresh_log.jsonl" {
		t.Fatalf("Storage.AuditFile = %q, want %q", cfg.Storage.AuditFile, "./data/custom_refresh_log.jsonl")
	}
}
