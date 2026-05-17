package cpaconvert

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/transferdata"
)

func TestConvertDirBuildsImportPayloadAndPreservesAPIKeys(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "auths"))
	mustMkdirAll(t, filepath.Join(root, "config"))

	writeFile(t, filepath.Join(root, "auths", "openai.json"), `{
  "type": "codex",
  "email": "user@example.com",
  "access_token": "access-1",
  "refresh_token": "refresh-1",
  "id_token": `+quoteJSONString(makeTestJWT(t, map[string]any{
		"email": "user@example.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "acct_1",
			"chatgpt_user_id":    "user_1",
			"chatgpt_plan_type":  "plus",
			"organizations": []map[string]any{
				{"id": "org_1", "is_default": true},
			},
		},
	}))+`,
  "proxy_url": "http://proxy.local:8080"
}`)
	writeFile(t, filepath.Join(root, "auths", "gemini.json"), `{
  "provider": "gemini",
  "metadata": {
    "project_id": "proj-1",
    "token": {
      "access_token": "ga-access",
      "refresh_token": "ga-refresh"
    }
  }
}`)
	writeFile(t, filepath.Join(root, "config", "config.yaml"), "api-keys:\n  - sk-one\n  - sk-two\n  - sk-one\n")

	result, err := ConvertDir(root)
	if err != nil {
		t.Fatalf("ConvertDir returned error: %v", err)
	}

	if got, want := len(result.DataPayload.Accounts), 2; got != want {
		t.Fatalf("accounts = %d, want %d", got, want)
	}
	if got, want := len(result.DataPayload.Proxies), 1; got != want {
		t.Fatalf("proxies = %d, want %d", got, want)
	}
	if got, want := len(result.PreservedAPIKeys), 2; got != want {
		t.Fatalf("preserved keys = %d, want %d", got, want)
	}
	var openAISeen bool
	var geminiSeen bool
	for _, account := range result.DataPayload.Accounts {
		switch account.Platform {
		case service.PlatformOpenAI:
			openAISeen = true
			if account.ProxyKey == nil || *account.ProxyKey == "" {
				t.Fatalf("expected proxy key on openai account")
			}
		case service.PlatformGemini:
			geminiSeen = true
		}
	}
	if !openAISeen {
		t.Fatal("expected one openai account in payload")
	}
	if !geminiSeen {
		t.Fatal("expected one gemini account in payload")
	}
	if result.Summary.AccountsSkipped != 0 {
		t.Fatalf("accounts skipped = %d, want 0", result.Summary.AccountsSkipped)
	}
}

func TestConvertDirSkipsDisabledAndServiceAccount(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "auths"))
	mustMkdirAll(t, filepath.Join(root, "config"))

	writeFile(t, filepath.Join(root, "auths", "disabled.json"), `{
  "type": "codex",
  "email": "disabled@example.com",
  "access_token": "access-1",
  "disabled": true
}`)
	writeFile(t, filepath.Join(root, "auths", "vertex.json"), `{
  "provider": "vertex",
  "metadata": {
    "project_id": "vertex-project",
    "service_account": {
      "type": "service_account",
      "client_email": "svc@example.com"
    }
  }
}`)
	writeFile(t, filepath.Join(root, "config", "config.yaml"), "api-keys: []\n")

	result, err := ConvertDir(root)
	if err != nil {
		t.Fatalf("ConvertDir returned error: %v", err)
	}

	if got := len(result.DataPayload.Accounts); got != 0 {
		t.Fatalf("accounts = %d, want 0", got)
	}
	if got, want := len(result.SkippedAccounts), 2; got != want {
		t.Fatalf("skipped accounts = %d, want %d", got, want)
	}
	if result.Summary.DisabledSkipped != 1 {
		t.Fatalf("disabled skipped = %d, want 1", result.Summary.DisabledSkipped)
	}
	if result.Summary.ServiceAcctSkipped != 1 {
		t.Fatalf("service account skipped = %d, want 1", result.Summary.ServiceAcctSkipped)
	}
}

func TestConvertDirAcceptsUTF8BOMFiles(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "auths"))
	mustMkdirAll(t, filepath.Join(root, "config"))

	writeFile(t, filepath.Join(root, "auths", "gemini.json"), "\uFEFF{\n  \"provider\": \"gemini\",\n  \"metadata\": {\n    \"project_id\": \"proj-bom\",\n    \"token\": {\n      \"access_token\": \"ga-access\"\n    }\n  }\n}")
	writeFile(t, filepath.Join(root, "config", "config.yaml"), "\uFEFFapi-keys:\n  - sk-bom\n")

	result, err := ConvertDir(root)
	if err != nil {
		t.Fatalf("ConvertDir returned error: %v", err)
	}

	if got, want := len(result.DataPayload.Accounts), 1; got != want {
		t.Fatalf("accounts = %d, want %d", got, want)
	}
	if got, want := len(result.PreservedAPIKeys), 1; got != want {
		t.Fatalf("preserved keys = %d, want %d", got, want)
	}
}

func TestWriteOutputsCreatesExpectedFiles(t *testing.T) {
	result := &Result{
		DataPayload: transferdata.DataPayload{
			Type:       "sub2api-data",
			Version:    1,
			ExportedAt: time.Now().UTC().Format(time.RFC3339),
			Proxies:    []transferdata.DataProxy{},
			Accounts:   []transferdata.DataAccount{},
		},
		PreservedAPIKeys: []PreservedAPIKey{{Name: "k1", Key: "sk-1", SHA256: hashAPIKey("sk-1"), Source: "config"}},
		SkippedAccounts:  []SkippedAccount{{FileName: "a.json", Reason: "skip"}},
		Warnings:         []string{"warn-1"},
		Summary:          Summary{AccountsSeen: 1},
	}

	outDir := filepath.Join(t.TempDir(), "out")
	if err := WriteOutputs(outDir, result); err != nil {
		t.Fatalf("WriteOutputs returned error: %v", err)
	}

	for _, name := range []string{
		"sub2api-import.json",
		"preserved-api-keys.json",
		"skipped-accounts.json",
		"conversion-report.json",
	} {
		if _, err := os.Stat(filepath.Join(outDir, name)); err != nil {
			t.Fatalf("expected output file %s: %v", name, err)
		}
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", path, err)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func makeTestJWT(t *testing.T, payload map[string]any) string {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal jwt payload: %v", err)
	}
	return "e30." + base64.RawURLEncoding.EncodeToString(data) + ".sig"
}

func quoteJSONString(value string) string {
	encoded, _ := json.Marshal(value)
	return strings.TrimSpace(string(encoded))
}
