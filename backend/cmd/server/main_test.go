package main

import (
	"testing"
	"time"
)

func TestCPAImportBootstrapTimeout(t *testing.T) {
	t.Run("default timeout is thirty minutes", func(t *testing.T) {
		t.Setenv("CPA_IMPORT_BOOTSTRAP_TIMEOUT_SECONDS", "")
		if got := cpaImportBootstrapTimeout(); got != 30*time.Minute {
			t.Fatalf("cpaImportBootstrapTimeout()=%s, want %s", got, 30*time.Minute)
		}
	})

	t.Run("valid override uses seconds from env", func(t *testing.T) {
		t.Setenv("CPA_IMPORT_BOOTSTRAP_TIMEOUT_SECONDS", "90")
		if got := cpaImportBootstrapTimeout(); got != 90*time.Second {
			t.Fatalf("cpaImportBootstrapTimeout()=%s, want %s", got, 90*time.Second)
		}
	})

	t.Run("invalid override falls back to default", func(t *testing.T) {
		t.Setenv("CPA_IMPORT_BOOTSTRAP_TIMEOUT_SECONDS", "bad")
		if got := cpaImportBootstrapTimeout(); got != 30*time.Minute {
			t.Fatalf("cpaImportBootstrapTimeout()=%s, want %s", got, 30*time.Minute)
		}
	})
}
