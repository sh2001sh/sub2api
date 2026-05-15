package cpaimport

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"gopkg.in/yaml.v3"
)

func TestSyncService_SyncAccountUpsert_WritesLegacyAuthFile(t *testing.T) {
	rootDir := t.TempDir()
	t.Setenv("CPA_IMPORT_ENABLED", "true")
	t.Setenv("CPA_IMPORT_SOURCE_DIR", rootDir)
	t.Setenv("GITSTORE_GIT_URL", "")

	syncService := NewSyncService(nil)
	account := &service.Account{
		ID:       42,
		Name:     "Codex primary",
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Status:   service.StatusActive,
		Credentials: map[string]any{
			"access_token":  "access-token",
			"refresh_token": "refresh-token",
			"id_token":      "id-token",
			"email":         "coder@example.com",
		},
		Extra: map[string]any{
			"legacy_cpa_provider": "codex",
			"legacy_cpa_file":     "team/codex-main.json",
			"legacy_cpa_prefix":   "team-a",
		},
	}

	if err := syncService.SyncAccountUpsert(context.Background(), account); err != nil {
		t.Fatalf("SyncAccountUpsert returned error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(rootDir, "auths", "team", "codex-main.json"))
	if err != nil {
		t.Fatalf("read synced auth file: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal synced auth file: %v", err)
	}

	if got := readString(doc, "type"); got != "codex" {
		t.Fatalf("expected provider codex, got %q", got)
	}
	if got := readString(doc, "prefix"); got != "team-a" {
		t.Fatalf("expected prefix team-a, got %q", got)
	}
	if got := readString(doc, "access_token"); got != "access-token" {
		t.Fatalf("expected access token to be written, got %q", got)
	}
	if got := readString(doc, "email"); got != "coder@example.com" {
		t.Fatalf("expected email to be written, got %q", got)
	}
	if disabled, _ := doc["disabled"].(bool); disabled {
		t.Fatalf("expected synced auth to remain enabled")
	}
}

func TestSyncService_SyncAccountDelete_RemovesLegacyAuthFile(t *testing.T) {
	rootDir := t.TempDir()
	t.Setenv("CPA_IMPORT_ENABLED", "true")
	t.Setenv("CPA_IMPORT_SOURCE_DIR", rootDir)
	t.Setenv("GITSTORE_GIT_URL", "")

	authDir := filepath.Join(rootDir, "auths")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatalf("create auth dir: %v", err)
	}
	authPath := filepath.Join(authDir, "sub2api-account-7.json")
	if err := os.WriteFile(authPath, []byte(`{"type":"claude"}`), 0o600); err != nil {
		t.Fatalf("seed auth file: %v", err)
	}

	syncService := NewSyncService(nil)
	account := &service.Account{
		ID:       7,
		Name:     "Claude account",
		Platform: service.PlatformAnthropic,
		Type:     service.AccountTypeOAuth,
		Status:   service.StatusActive,
	}

	if err := syncService.SyncAccountDelete(context.Background(), account); err != nil {
		t.Fatalf("SyncAccountDelete returned error: %v", err)
	}
	if _, err := os.Stat(authPath); !os.IsNotExist(err) {
		t.Fatalf("expected auth file to be removed, stat err=%v", err)
	}
}

func TestSyncService_SyncLegacyAPIKey_UpdatesConfigYAML(t *testing.T) {
	rootDir := t.TempDir()
	t.Setenv("CPA_IMPORT_ENABLED", "true")
	t.Setenv("CPA_IMPORT_SOURCE_DIR", rootDir)
	t.Setenv("GITSTORE_GIT_URL", "")

	configDir := filepath.Join(rootDir, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("feature-flag: true\napi-keys:\n  - old-key\n"), 0o600); err != nil {
		t.Fatalf("seed config file: %v", err)
	}

	syncService := NewSyncService(nil)
	owner := &service.User{ID: 1, Email: legacyUserEmail}
	apiKey := &service.APIKey{ID: 99, UserID: owner.ID, Key: "new-key"}

	if err := syncService.SyncAPIKeyUpsert(context.Background(), apiKey, owner); err != nil {
		t.Fatalf("SyncAPIKeyUpsert returned error: %v", err)
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config after upsert: %v", err)
	}
	var cfg map[string]any
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse config after upsert: %v", err)
	}
	if flag, _ := cfg["feature-flag"].(bool); !flag {
		t.Fatalf("expected unrelated config fields to remain intact")
	}
	keys, ok := cfg["api-keys"].([]any)
	if !ok || len(keys) != 2 {
		t.Fatalf("expected two api keys after sync, got %#v", cfg["api-keys"])
	}

	if err := syncService.SyncAPIKeyDelete(context.Background(), apiKey, owner); err != nil {
		t.Fatalf("SyncAPIKeyDelete returned error: %v", err)
	}
	raw, err = os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config after delete: %v", err)
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse config after delete: %v", err)
	}
	keys, ok = cfg["api-keys"].([]any)
	if !ok || len(keys) != 1 || keys[0] != "old-key" {
		t.Fatalf("expected only old-key to remain after delete, got %#v", cfg["api-keys"])
	}
}

func TestSyncService_SyncAccountUpsert_PushesToGitRemote(t *testing.T) {
	rootDir := t.TempDir()
	remoteDir := filepath.Join(rootDir, "remote.git")
	workDir := filepath.Join(rootDir, "seed")
	verifyDir := filepath.Join(rootDir, "verify")

	runGitTest(t, "", "init", "--bare", remoteDir)
	runGitTest(t, "", "clone", remoteDir, workDir)
	runGitTest(t, workDir, "config", "user.name", "tester")
	runGitTest(t, workDir, "config", "user.email", "tester@example.com")
	mustWriteTestFile(t, filepath.Join(workDir, "config", "config.yaml"), "api-keys: []\n")
	runGitTest(t, workDir, "add", ".")
	runGitTest(t, workDir, "commit", "-m", "seed")
	runGitTest(t, workDir, "push", "origin", "HEAD")

	t.Setenv("CPA_IMPORT_ENABLED", "true")
	t.Setenv("CPA_IMPORT_SOURCE_DIR", "")
	t.Setenv("GITSTORE_GIT_URL", remoteDir)
	t.Setenv("GITSTORE_GIT_USERNAME", "")
	t.Setenv("GITSTORE_GIT_TOKEN", "")
	t.Setenv("GITSTORE_GIT_BRANCH", "")

	syncService := NewSyncService(nil)
	account := &service.Account{
		ID:       88,
		Name:     "Git-backed codex",
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Status:   service.StatusActive,
		Credentials: map[string]any{
			"access_token":  "git-access",
			"refresh_token": "git-refresh",
			"email":         "git@example.com",
		},
		Extra: map[string]any{
			"legacy_cpa_provider": "codex",
			"legacy_cpa_file":     "codex-git.json",
		},
	}

	if err := syncService.SyncAccountUpsert(context.Background(), account); err != nil {
		t.Fatalf("SyncAccountUpsert returned error: %v", err)
	}

	runGitTest(t, "", "clone", remoteDir, verifyDir)
	data, err := os.ReadFile(filepath.Join(verifyDir, "auths", "codex-git.json"))
	if err != nil {
		t.Fatalf("read pushed auth file: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal pushed auth file: %v", err)
	}
	if got := readString(doc, "access_token"); got != "git-access" {
		t.Fatalf("expected pushed access token git-access, got %q", got)
	}
	if got := readString(doc, "refresh_token"); got != "git-refresh" {
		t.Fatalf("expected pushed refresh token git-refresh, got %q", got)
	}
}

func TestSyncService_SyncAccountUpsert_StripsAuthsPrefixFromLegacyFile(t *testing.T) {
	rootDir := t.TempDir()
	t.Setenv("CPA_IMPORT_ENABLED", "true")
	t.Setenv("CPA_IMPORT_SOURCE_DIR", rootDir)
	t.Setenv("GITSTORE_GIT_URL", "")

	syncService := NewSyncService(nil)
	account := &service.Account{
		ID:       123,
		Name:     "Prefixed file account",
		Platform: service.PlatformAnthropic,
		Type:     service.AccountTypeOAuth,
		Status:   service.StatusActive,
		Credentials: map[string]any{
			"access_token": "prefixed-token",
		},
		Extra: map[string]any{
			"legacy_cpa_provider": "claude",
			"legacy_cpa_file":     "auths/nested/prefixed.json",
		},
	}

	if err := syncService.SyncAccountUpsert(context.Background(), account); err != nil {
		t.Fatalf("SyncAccountUpsert returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(rootDir, "auths", "nested", "prefixed.json")); err != nil {
		t.Fatalf("expected normalized auth path to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, "auths", "auths", "nested", "prefixed.json")); !os.IsNotExist(err) {
		t.Fatalf("expected duplicated auths/auths path to stay absent, stat err=%v", err)
	}
}

func TestSyncService_SyncLegacyAPIKey_PushesToGitRemote(t *testing.T) {
	rootDir := t.TempDir()
	remoteDir := filepath.Join(rootDir, "remote.git")
	workDir := filepath.Join(rootDir, "seed")
	verifyDir := filepath.Join(rootDir, "verify")

	runGitTest(t, "", "init", "--bare", remoteDir)
	runGitTest(t, "", "clone", remoteDir, workDir)
	runGitTest(t, workDir, "config", "user.name", "tester")
	runGitTest(t, workDir, "config", "user.email", "tester@example.com")
	mustWriteTestFile(t, filepath.Join(workDir, "config", "config.yaml"), "api-keys:\n  - old-key\n")
	runGitTest(t, workDir, "add", ".")
	runGitTest(t, workDir, "commit", "-m", "seed")
	runGitTest(t, workDir, "push", "origin", "HEAD")

	t.Setenv("CPA_IMPORT_ENABLED", "true")
	t.Setenv("CPA_IMPORT_SOURCE_DIR", "")
	t.Setenv("GITSTORE_GIT_URL", remoteDir)
	t.Setenv("GITSTORE_GIT_USERNAME", "")
	t.Setenv("GITSTORE_GIT_TOKEN", "")
	t.Setenv("GITSTORE_GIT_BRANCH", "")

	syncService := NewSyncService(nil)
	owner := &service.User{ID: 1, Email: legacyUserEmail}
	apiKey := &service.APIKey{ID: 501, UserID: owner.ID, Key: "new-remote-key"}

	if err := syncService.SyncAPIKeyUpsert(context.Background(), apiKey, owner); err != nil {
		t.Fatalf("SyncAPIKeyUpsert returned error: %v", err)
	}

	runGitTest(t, "", "clone", remoteDir, verifyDir)
	configPath := filepath.Join(verifyDir, "config", "config.yaml")
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read pushed config file: %v", err)
	}

	var cfg map[string]any
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse pushed config file: %v", err)
	}
	keys, ok := cfg["api-keys"].([]any)
	if !ok || len(keys) != 2 {
		t.Fatalf("expected two api keys after remote sync, got %#v", cfg["api-keys"])
	}

	if err := syncService.SyncAPIKeyDelete(context.Background(), apiKey, owner); err != nil {
		t.Fatalf("SyncAPIKeyDelete returned error: %v", err)
	}

	verifyDir2 := filepath.Join(rootDir, "verify-delete")
	runGitTest(t, "", "clone", remoteDir, verifyDir2)
	raw, err = os.ReadFile(filepath.Join(verifyDir2, "config", "config.yaml"))
	if err != nil {
		t.Fatalf("read config after remote delete: %v", err)
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse config after remote delete: %v", err)
	}
	keys, ok = cfg["api-keys"].([]any)
	if !ok || len(keys) != 1 || keys[0] != "old-key" {
		t.Fatalf("expected old-key only after remote delete, got %#v", cfg["api-keys"])
	}
}

func runGitTest(t *testing.T, workDir string, args ...string) {
	t.Helper()

	cmdArgs := args
	if strings.TrimSpace(workDir) != "" {
		cmdArgs = append([]string{"-C", workDir}, args...)
	}
	cmd := exec.Command("git", cmdArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
}

func mustWriteTestFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent dir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
