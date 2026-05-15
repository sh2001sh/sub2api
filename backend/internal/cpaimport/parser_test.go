package cpaimport

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestBuildImportSpec_GeminiOAuthTokenStorage(t *testing.T) {
	auth := LegacyAuth{
		ID:       "gemini-user",
		Provider: "gemini",
		Metadata: map[string]any{
			"project_id": "proj-1",
			"token": map[string]any{
				"access_token":  "access-1",
				"refresh_token": "refresh-1",
			},
		},
	}

	spec, warnings, err := buildImportSpec(auth)
	if err != nil {
		t.Fatalf("buildImportSpec returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if spec.Platform != service.PlatformGemini {
		t.Fatalf("unexpected platform: %q", spec.Platform)
	}
	if spec.AccountType != service.AccountTypeOAuth {
		t.Fatalf("unexpected account type: %q", spec.AccountType)
	}
	if spec.Credentials["oauth_type"] != "code_assist" {
		t.Fatalf("expected oauth_type=code_assist, got %#v", spec.Credentials["oauth_type"])
	}
	if spec.Credentials["access_token"] != "access-1" {
		t.Fatalf("expected access token to be flattened")
	}
}

func TestBuildImportSpec_OpenAIAPIKey(t *testing.T) {
	auth := LegacyAuth{
		ID:       "openai-apikey",
		Provider: "codex",
		Attributes: map[string]string{
			"api_key":   "sk-legacy",
			"base_url":  "https://api.openai.com/v1",
			"plan_type": "plus",
		},
	}

	spec, warnings, err := buildImportSpec(auth)
	if err != nil {
		t.Fatalf("buildImportSpec returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if spec.Platform != service.PlatformOpenAI {
		t.Fatalf("unexpected platform: %q", spec.Platform)
	}
	if spec.AccountType != service.AccountTypeAPIKey {
		t.Fatalf("unexpected account type: %q", spec.AccountType)
	}
	if spec.Credentials["api_key"] != "sk-legacy" {
		t.Fatalf("expected api_key to be copied")
	}
}

func TestBuildImportSpec_OpenAIOAuthIDTokenEnrichment(t *testing.T) {
	auth := LegacyAuth{
		ID:       "openai-oauth",
		Provider: "openai",
		Metadata: map[string]any{
			"id_token": makeTestJWT(t, map[string]any{
				"email": "legacy@example.com",
				"exp":   time.Now().Add(time.Hour).Unix(),
				"https://api.openai.com/auth": map[string]any{
					"chatgpt_account_id": "acct_legacy",
					"chatgpt_user_id":    "user_legacy",
					"chatgpt_plan_type":  "plus",
					"organizations": []map[string]any{
						{
							"id":         "org_default",
							"is_default": true,
						},
					},
				},
			}),
		},
	}

	spec, warnings, err := buildImportSpec(auth)
	if err != nil {
		t.Fatalf("buildImportSpec returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if spec.AccountType != service.AccountTypeOAuth {
		t.Fatalf("unexpected account type: %q", spec.AccountType)
	}
	if spec.Credentials["email"] != "legacy@example.com" {
		t.Fatalf("expected email from id_token, got %#v", spec.Credentials["email"])
	}
	if spec.Credentials["chatgpt_account_id"] != "acct_legacy" {
		t.Fatalf("expected chatgpt_account_id from id_token")
	}
	if spec.Credentials["chatgpt_user_id"] != "user_legacy" {
		t.Fatalf("expected chatgpt_user_id from id_token")
	}
	if spec.Credentials["organization_id"] != "org_default" {
		t.Fatalf("expected organization_id from id_token")
	}
	if spec.Credentials["plan_type"] != "plus" {
		t.Fatalf("expected plan_type from id_token")
	}
}

func TestBuildImportSpec_VertexServiceAccount(t *testing.T) {
	auth := LegacyAuth{
		ID:       "vertex-service-account",
		Provider: "vertex",
		Metadata: map[string]any{
			"project_id": "vertex-project",
			"location":   "us-central1",
			"service_account": map[string]any{
				"type":         "service_account",
				"project_id":   "vertex-project",
				"private_key":  "-----BEGIN PRIVATE KEY-----\\nabc\\n-----END PRIVATE KEY-----\\n",
				"client_email": "svc@example.iam.gserviceaccount.com",
			},
		},
	}

	spec, warnings, err := buildImportSpec(auth)
	if err != nil {
		t.Fatalf("buildImportSpec returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if spec.Platform != service.PlatformGemini {
		t.Fatalf("unexpected platform: %q", spec.Platform)
	}
	if spec.AccountType != service.AccountTypeServiceAccount {
		t.Fatalf("unexpected account type: %q", spec.AccountType)
	}
	serviceAccount, ok := spec.Credentials["service_account"].(map[string]any)
	if !ok || serviceAccount["client_email"] != "svc@example.iam.gserviceaccount.com" {
		t.Fatalf("expected service_account credentials to be preserved, got %#v", spec.Credentials["service_account"])
	}
}

func TestNormalizeProxyURL_DirectMeansNoProxy(t *testing.T) {
	normalized, parsed, err := normalizeProxyURL("direct")
	if err != nil {
		t.Fatalf("normalizeProxyURL returned error: %v", err)
	}
	if normalized != "" || parsed != nil {
		t.Fatalf("expected direct proxy marker to disable proxy")
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
