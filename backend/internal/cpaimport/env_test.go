package cpaimport

import "os"

import "testing"

func TestLoadConfigFromEnv_DisabledByDefault(t *testing.T) {
	t.Setenv("CPA_IMPORT_ENABLED", "")
	t.Setenv("CPA_IMPORT_SOURCE_DIR", "")
	t.Setenv("GITSTORE_GIT_URL", "")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv returned error: %v", err)
	}
	if cfg.Enabled {
		t.Fatalf("expected import to be disabled by default")
	}
}

func TestLoadConfigFromEnv_AutoEnablesWhenGitStoreProvided(t *testing.T) {
	_ = os.Unsetenv("CPA_IMPORT_ENABLED")
	t.Setenv("GITSTORE_GIT_URL", "https://example.com/repo.git")
	t.Setenv("GITSTORE_GIT_USERNAME", "tester")
	t.Setenv("GITSTORE_GIT_TOKEN", "secret")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv returned error: %v", err)
	}
	if !cfg.Enabled {
		t.Fatalf("expected import to auto-enable when GITSTORE_GIT_URL is set")
	}
}

func TestLoadConfigFromEnv_ExplicitFalseDisablesAutoEnable(t *testing.T) {
	t.Setenv("CPA_IMPORT_ENABLED", "false")
	t.Setenv("GITSTORE_GIT_URL", "https://example.com/repo.git")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv returned error: %v", err)
	}
	if cfg.Enabled {
		t.Fatalf("expected explicit false to disable import")
	}
}

func TestLoadConfigFromEnv_RequiresSourceWhenEnabled(t *testing.T) {
	t.Setenv("CPA_IMPORT_ENABLED", "true")
	t.Setenv("CPA_IMPORT_SOURCE_DIR", "")
	t.Setenv("GITSTORE_GIT_URL", "")

	if _, err := LoadConfigFromEnv(); err == nil {
		t.Fatalf("expected error when import is enabled without a source")
	}
}

func TestLoadConfigFromEnv_UsesGitStoreVariables(t *testing.T) {
	t.Setenv("CPA_IMPORT_ENABLED", "1")
	t.Setenv("GITSTORE_GIT_URL", "https://example.com/repo.git")
	t.Setenv("GITSTORE_GIT_USERNAME", "tester")
	t.Setenv("GITSTORE_GIT_TOKEN", "secret")
	t.Setenv("GITSTORE_GIT_BRANCH", "main")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv returned error: %v", err)
	}
	if cfg.GitURL != "https://example.com/repo.git" {
		t.Fatalf("unexpected git url: %q", cfg.GitURL)
	}
	if cfg.GitUser != "tester" || cfg.GitToken != "secret" || cfg.GitBranch != "main" {
		t.Fatalf("unexpected git config: %+v", cfg)
	}
}
