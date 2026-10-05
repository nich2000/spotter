package main

import (
	"context"
	"testing"
)

func TestConfigurationAndFailure(t *testing.T) {
	t.Setenv("TEST_SPOTTER_ENV", "")
	if env("TEST_SPOTTER_ENV", "fallback") != "fallback" {
		t.Fatal("default")
	}
	t.Setenv("TEST_SPOTTER_ENV", "value")
	if env("TEST_SPOTTER_ENV", "fallback") != "value" {
		t.Fatal("override")
	}
	t.Setenv("DATABASE_URL", "://invalid")
	if run(context.Background(), "api") == nil {
		t.Fatal("invalid database")
	}
	if migrateLegacy("missing-file", false) == nil {
		t.Fatal("missing migration")
	}
}
