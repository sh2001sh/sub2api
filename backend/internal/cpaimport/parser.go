package cpaimport

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"gopkg.in/yaml.v3"
)

func loadLegacyData(rootDir string) ([]LegacyAuth, []string, error) {
	auths, err := loadLegacyAuths(filepath.Join(rootDir, "auths"))
	if err != nil {
		return nil, nil, err
	}
	keys, err := loadLegacyAPIKeys(filepath.Join(rootDir, "config", "config.yaml"))
	if err != nil {
		return nil, nil, err
	}
	return auths, keys, nil
}

func loadLegacyAuths(authDir string) ([]LegacyAuth, error) {
	entries, err := os.ReadDir(authDir)
	if err != nil {
		return nil, fmt.Errorf("read legacy auth dir: %w", err)
	}
	auths := make([]LegacyAuth, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		fullPath := filepath.Join(authDir, entry.Name())
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("read legacy auth file %s: %w", entry.Name(), err)
		}
		auth, err := parseLegacyAuth(data, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("parse legacy auth file %s: %w", entry.Name(), err)
		}
		auths = append(auths, auth)
	}
	return auths, nil
}

func parseLegacyAuth(data []byte, fileName string) (LegacyAuth, error) {
	var raw map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		return LegacyAuth{}, err
	}

	auth := LegacyAuth{
		Attributes: map[string]string{},
		Metadata:   map[string]any{},
		FileName:   fileName,
		Raw:        raw,
	}
	if v, ok := raw["id"].(string); ok {
		auth.ID = strings.TrimSpace(v)
	}
	if v, ok := raw["provider"].(string); ok {
		auth.Provider = strings.TrimSpace(v)
	}
	if v, ok := raw["prefix"].(string); ok {
		auth.Prefix = strings.TrimSpace(v)
	}
	if v, ok := raw["label"].(string); ok {
		auth.Label = strings.TrimSpace(v)
	}
	if v, ok := raw["disabled"].(bool); ok {
		auth.Disabled = v
	}
	if v, ok := raw["proxy_url"].(string); ok {
		auth.ProxyURL = strings.TrimSpace(v)
	}
	if attrs, ok := raw["attributes"].(map[string]any); ok {
		for key, value := range attrs {
			if value == nil {
				continue
			}
			auth.Attributes[key] = strings.TrimSpace(fmt.Sprint(value))
		}
	}
	if metadata, ok := raw["metadata"].(map[string]any); ok {
		auth.Metadata = metadata
	}
	if auth.ID == "" {
		auth.ID = strings.TrimSuffix(fileName, filepath.Ext(fileName))
	}
	return auth, nil
}

func loadLegacyAPIKeys(configPath string) ([]string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("read legacy config: %w", err)
	}
	var cfg struct {
		APIKeys []string `yaml:"api-keys"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse legacy config yaml: %w", err)
	}
	seen := make(map[string]struct{}, len(cfg.APIKeys))
	keys := make([]string, 0, len(cfg.APIKeys))
	for _, raw := range cfg.APIKeys {
		key := strings.TrimSpace(raw)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	return keys, nil
}

func buildImportSpec(auth LegacyAuth) (*ImportAccountSpec, []string, error) {
	provider := normalizeProvider(auth.Provider)
	platform, accountType, warnings, err := resolvePlatformAndType(provider, auth)
	if err != nil {
		return nil, warnings, err
	}

	credentials := map[string]any{}
	extra := map[string]any{
		"legacy_cpa_id":       auth.ID,
		"legacy_cpa_file":     auth.FileName,
		"legacy_cpa_provider": provider,
		"legacy_cpa_prefix":   auth.Prefix,
		"legacy_cpa_proxy":    auth.ProxyURL,
		"legacy_cpa_label":    auth.Label,
		"legacy_cpa_raw":      auth.Raw,
	}

	copyAttributeStrings(credentials, auth.Attributes,
		"api_key",
		"base_url",
		"user_agent",
		"oauth_type",
		"project_id",
		"location",
		"provider_key",
		"compat_name",
		"plan_type",
		"chatgpt_account_id",
		"chatgpt_user_id",
		"organization_id",
		"claude_user_id",
		"anthropic_user_id",
		"auth_mode",
	)
	copyMetadataStrings(credentials, auth.Metadata,
		"access_token",
		"refresh_token",
		"id_token",
		"email",
		"project_id",
		"location",
		"user_agent",
		"plan_type",
		"chatgpt_account_id",
		"chatgpt_user_id",
		"organization_id",
		"claude_user_id",
		"anthropic_user_id",
	)

	if accountID := readString(auth.Metadata, "account_id"); accountID != "" && readStringFromAny(credentials["chatgpt_account_id"]) == "" {
		credentials["chatgpt_account_id"] = accountID
	}

	if tokenMap, ok := auth.Metadata["token"].(map[string]any); ok {
		copyMetadataStrings(credentials, tokenMap,
			"access_token",
			"refresh_token",
			"id_token",
			"token_type",
			"client_id",
			"client_secret",
			"token_uri",
		)
		if scopes, ok := tokenMap["scopes"]; ok {
			credentials["scopes"] = scopes
		}
	}

	if serviceAccount, ok := auth.Metadata["service_account"].(map[string]any); ok && len(serviceAccount) > 0 {
		credentials["service_account"] = serviceAccount
	}

	if expiresAt := normalizeLegacyExpiry(auth.Metadata); expiresAt != "" {
		credentials["expires_at"] = expiresAt
	}

	switch platform {
	case service.PlatformOpenAI:
		enrichOpenAICredentials(credentials)
	case service.PlatformGemini:
		if _, exists := credentials["oauth_type"]; !exists && accountType == service.AccountTypeOAuth {
			switch provider {
			case "aistudio":
				credentials["oauth_type"] = "ai_studio"
			case "gemini", "gemini-cli":
				if readStringFromAny(credentials["project_id"]) != "" {
					credentials["oauth_type"] = "code_assist"
				} else {
					credentials["oauth_type"] = "google_one"
				}
			}
		}
	}

	note := buildAccountNote(auth, provider)
	spec := &ImportAccountSpec{
		LegacyID:    auth.ID,
		Name:        buildAccountName(auth, provider),
		Platform:    platform,
		AccountType: accountType,
		Status:      service.StatusActive,
		Notes:       &note,
		ProxyURL:    strings.TrimSpace(auth.ProxyURL),
		Credentials: credentials,
		Extra:       extra,
	}
	if auth.Disabled {
		spec.Status = service.StatusDisabled
	}
	spec.Checksum = checksumImportSpec(spec)
	return spec, warnings, nil
}

func resolvePlatformAndType(provider string, auth LegacyAuth) (string, string, []string, error) {
	switch provider {
	case "claude", "anthropic":
		if hasAttribute(auth, "api_key") {
			return service.PlatformAnthropic, service.AccountTypeAPIKey, nil, nil
		}
		return service.PlatformAnthropic, service.AccountTypeOAuth, nil, nil
	case "codex", "openai":
		if hasAttribute(auth, "api_key") {
			return service.PlatformOpenAI, service.AccountTypeAPIKey, nil, nil
		}
		return service.PlatformOpenAI, service.AccountTypeOAuth, nil, nil
	case "gemini", "gemini-cli", "aistudio":
		if hasAttribute(auth, "api_key") {
			return service.PlatformGemini, service.AccountTypeAPIKey, nil, nil
		}
		return service.PlatformGemini, service.AccountTypeOAuth, nil, nil
	case "vertex":
		if hasAttribute(auth, "api_key") {
			return service.PlatformGemini, service.AccountTypeAPIKey, nil, nil
		}
		if _, ok := auth.Metadata["service_account"].(map[string]any); ok {
			return service.PlatformGemini, service.AccountTypeServiceAccount, nil, nil
		}
		return service.PlatformGemini, service.AccountTypeOAuth, []string{"vertex auth imported as oauth because no api_key/service_account payload was detected"}, nil
	case "antigravity":
		if hasAttribute(auth, "api_key") {
			return service.PlatformAntigravity, service.AccountTypeAPIKey, nil, nil
		}
		return service.PlatformAntigravity, service.AccountTypeOAuth, nil, nil
	default:
		return "", "", nil, fmt.Errorf("unsupported legacy provider %q", provider)
	}
}

func checksumImportSpec(spec *ImportAccountSpec) string {
	payload := map[string]any{
		"name":         spec.Name,
		"platform":     spec.Platform,
		"account_type": spec.AccountType,
		"status":       spec.Status,
		"notes":        spec.Notes,
		"proxy_url":    spec.ProxyURL,
		"credentials":  spec.Credentials,
		"extra":        spec.Extra,
	}
	data, _ := json.Marshal(payload)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func hashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(key)))
	return hex.EncodeToString(sum[:])
}

func normalizeProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

func hasAttribute(auth LegacyAuth, key string) bool {
	return strings.TrimSpace(auth.Attributes[key]) != ""
}

func buildAccountName(auth LegacyAuth, provider string) string {
	if auth.Label != "" {
		return auth.Label
	}
	return fmt.Sprintf("cpa-%s-%s", provider, sanitizeNameToken(auth.ID))
}

func buildAccountNote(auth LegacyAuth, provider string) string {
	return fmt.Sprintf("Imported from CPA auth %s (%s)", auth.FileName, provider)
}

func sanitizeNameToken(raw string) string {
	token := strings.TrimSpace(raw)
	token = strings.ReplaceAll(token, " ", "-")
	token = strings.ReplaceAll(token, "/", "-")
	token = strings.ReplaceAll(token, "\\", "-")
	if token == "" {
		return "unknown"
	}
	return token
}

func copyAttributeStrings(dst map[string]any, src map[string]string, keys ...string) {
	for _, key := range keys {
		if value := strings.TrimSpace(src[key]); value != "" {
			dst[key] = value
		}
	}
}

func copyMetadataStrings(dst map[string]any, src map[string]any, keys ...string) {
	for _, key := range keys {
		if value := readString(src, key); value != "" {
			dst[key] = value
		}
	}
}

func readString(src map[string]any, key string) string {
	if src == nil {
		return ""
	}
	return readStringFromAny(src[key])
}

func readStringFromAny(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func normalizeLegacyExpiry(metadata map[string]any) string {
	if metadata == nil {
		return ""
	}
	for _, key := range []string{"expires_at", "expired"} {
		if value := readString(metadata, key); value != "" {
			return value
		}
	}
	if tokenMap, ok := metadata["token"].(map[string]any); ok {
		for _, key := range []string{"expiry", "expires_at"} {
			if value := readString(tokenMap, key); value != "" {
				return value
			}
		}
	}
	return ""
}

func enrichOpenAICredentials(credentials map[string]any) {
	idToken := readStringFromAny(credentials["id_token"])
	if idToken == "" {
		return
	}
	claims, err := openai.ParseIDToken(idToken)
	if err != nil {
		return
	}
	info := claims.GetUserInfo()
	if info == nil {
		return
	}
	if readStringFromAny(credentials["email"]) == "" && info.Email != "" {
		credentials["email"] = info.Email
	}
	if readStringFromAny(credentials["chatgpt_account_id"]) == "" && info.ChatGPTAccountID != "" {
		credentials["chatgpt_account_id"] = info.ChatGPTAccountID
	}
	if readStringFromAny(credentials["chatgpt_user_id"]) == "" && info.ChatGPTUserID != "" {
		credentials["chatgpt_user_id"] = info.ChatGPTUserID
	}
	if readStringFromAny(credentials["organization_id"]) == "" && info.OrganizationID != "" {
		credentials["organization_id"] = info.OrganizationID
	}
	if readStringFromAny(credentials["plan_type"]) == "" && info.PlanType != "" {
		credentials["plan_type"] = info.PlanType
	}
}

func normalizeProxyURL(raw string) (string, *url.URL, error) {
	value := strings.TrimSpace(raw)
	if value == "" || strings.EqualFold(value, "direct") {
		return "", nil, nil
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", nil, err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", nil, fmt.Errorf("missing scheme or host")
	}
	return parsed.String(), parsed, nil
}
