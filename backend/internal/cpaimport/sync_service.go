package cpaimport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"gopkg.in/yaml.v3"
)

const (
	legacyConfigRelativePath = "config/config.yaml"
	legacyAuthsDirName       = "auths"
)

type SyncRuntime struct {
	Service *SyncService
}

type SyncService struct {
	proxyRepo service.ProxyRepository
	mu        sync.Mutex
}

func NewSyncService(proxyRepo service.ProxyRepository) *SyncService {
	return &SyncService{proxyRepo: proxyRepo}
}

func ConfigureServiceSync(
	adminService service.AdminService,
	apiKeyService *service.APIKeyService,
	syncService *SyncService,
) *SyncRuntime {
	service.SetGlobalCPAStoreSyncer(syncService)
	if setter, ok := adminService.(interface{ SetCPAStoreSyncer(service.CPAStoreSyncer) }); ok {
		setter.SetCPAStoreSyncer(syncService)
	}
	if apiKeyService != nil {
		apiKeyService.SetCPAStoreSyncer(syncService)
	}
	return &SyncRuntime{Service: syncService}
}

func (s *SyncService) SyncAccountUpsert(ctx context.Context, account *service.Account) error {
	if account == nil || !isCPACompatibleAccount(account) {
		return nil
	}
	return s.withWritableStore(ctx, func(rootDir string, cfg Config) ([]string, string, error) {
		if err := ensureLegacyStoreLayout(rootDir); err != nil {
			return nil, "", err
		}
		fileName := legacyAuthFileName(account)
		authPath, err := resolveStorePath(rootDir, legacyAuthsDirName, fileName)
		if err != nil {
			return nil, "", err
		}
		doc, err := s.buildLegacyAuthDocument(ctx, account)
		if err != nil {
			return nil, "", err
		}
		changed, err := writeJSONIfChanged(authPath, doc)
		if err != nil {
			return nil, "", err
		}
		if !changed {
			return nil, "", nil
		}
		return []string{filepath.ToSlash(filepath.Join(legacyAuthsDirName, fileName))},
			fmt.Sprintf("Sync CPA auth %s", strings.TrimSpace(account.Name)),
			nil
	})
}

func (s *SyncService) SyncAccountDelete(ctx context.Context, account *service.Account) error {
	if account == nil || !isCPACompatibleAccount(account) {
		return nil
	}
	return s.withWritableStore(ctx, func(rootDir string, cfg Config) ([]string, string, error) {
		fileName := legacyAuthFileName(account)
		authPath, err := resolveStorePath(rootDir, legacyAuthsDirName, fileName)
		if err != nil {
			return nil, "", err
		}
		changed, err := deleteFileIfExists(authPath)
		if err != nil {
			return nil, "", err
		}
		if !changed {
			return nil, "", nil
		}
		return []string{filepath.ToSlash(filepath.Join(legacyAuthsDirName, fileName))},
			fmt.Sprintf("Delete CPA auth %s", strings.TrimSpace(account.Name)),
			nil
	})
}

func (s *SyncService) SyncAPIKeyUpsert(ctx context.Context, apiKey *service.APIKey, owner *service.User) error {
	if !shouldSyncLegacyAPIKey(apiKey, owner) {
		return nil
	}
	return s.withWritableStore(ctx, func(rootDir string, cfg Config) ([]string, string, error) {
		if err := ensureLegacyStoreLayout(rootDir); err != nil {
			return nil, "", err
		}
		configPath, err := resolveStorePath(rootDir, legacyConfigRelativePath)
		if err != nil {
			return nil, "", err
		}
		changed, err := updateLegacyConfigAPIKeys(configPath, apiKey.Key, true)
		if err != nil {
			return nil, "", err
		}
		if !changed {
			return nil, "", nil
		}
		return []string{filepath.ToSlash(legacyConfigRelativePath)},
			fmt.Sprintf("Sync legacy CPA API key %d", apiKey.ID),
			nil
	})
}

func (s *SyncService) SyncAPIKeyDelete(ctx context.Context, apiKey *service.APIKey, owner *service.User) error {
	if !shouldSyncLegacyAPIKey(apiKey, owner) {
		return nil
	}
	return s.withWritableStore(ctx, func(rootDir string, cfg Config) ([]string, string, error) {
		configPath, err := resolveStorePath(rootDir, legacyConfigRelativePath)
		if err != nil {
			return nil, "", err
		}
		changed, err := updateLegacyConfigAPIKeys(configPath, apiKey.Key, false)
		if err != nil {
			return nil, "", err
		}
		if !changed {
			return nil, "", nil
		}
		return []string{filepath.ToSlash(legacyConfigRelativePath)},
			fmt.Sprintf("Delete legacy CPA API key %d", apiKey.ID),
			nil
	})
}

func (s *SyncService) withWritableStore(
	ctx context.Context,
	fn func(rootDir string, cfg Config) ([]string, string, error),
) error {
	cfg, err := LoadConfigFromEnv()
	if err != nil || !cfg.Enabled {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	rootDir := strings.TrimSpace(cfg.SourceDir)
	cleanup := func() {}
	if strings.TrimSpace(cfg.GitURL) != "" {
		snap, err := cloneGitSnapshot(ctx, cfg)
		if err != nil {
			return err
		}
		rootDir = snap.rootDir
		cleanup = snap.cleanup
	}
	defer cleanup()

	if rootDir == "" {
		return fmt.Errorf("CPA sync store root is empty")
	}

	changedPaths, commitMessage, err := fn(rootDir, cfg)
	if err != nil {
		return err
	}
	if len(changedPaths) == 0 || strings.TrimSpace(cfg.GitURL) == "" {
		return nil
	}
	return persistGitWritableSnapshot(ctx, rootDir, cfg, changedPaths, commitMessage)
}

func (s *SyncService) buildLegacyAuthDocument(ctx context.Context, account *service.Account) (map[string]any, error) {
	doc := baseLegacyAuthDocument(account)
	provider := determineLegacyProvider(account)
	if provider == "" {
		return nil, fmt.Errorf("account %d is not CPA-compatible", account.ID)
	}

	doc["type"] = provider
	doc["label"] = strings.TrimSpace(account.Name)
	doc["disabled"] = account.Status != service.StatusActive

	if prefix := firstNonEmptyValue(account.GetExtraString("legacy_cpa_prefix"), readString(doc, "prefix")); prefix != "" {
		doc["prefix"] = prefix
	} else {
		delete(doc, "prefix")
	}

	proxyURL, err := s.resolveProxyURL(ctx, account)
	if err != nil {
		return nil, err
	}
	if proxyURL != "" {
		doc["proxy_url"] = proxyURL
	} else {
		delete(doc, "proxy_url")
	}

	if notes := normalizedNotes(account.Notes); notes != "" {
		doc["sub2api_notes"] = notes
	} else {
		delete(doc, "sub2api_notes")
	}

	for _, key := range []string{
		"access_token",
		"refresh_token",
		"id_token",
		"email",
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
		"client_id",
		"client_secret",
		"token_uri",
		"token_type",
		"expires_at",
	} {
		applyCredentialValue(doc, account.Credentials, key)
	}

	if serviceAccount, ok := cloneJSONValue(account.Credentials["service_account"]).(map[string]any); ok && len(serviceAccount) > 0 {
		doc["service_account"] = serviceAccount
	} else {
		delete(doc, "service_account")
	}

	return doc, nil
}

func (s *SyncService) resolveProxyURL(ctx context.Context, account *service.Account) (string, error) {
	if account == nil || account.ProxyID == nil || *account.ProxyID <= 0 || s.proxyRepo == nil {
		return "", nil
	}
	proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID)
	if err != nil {
		return "", fmt.Errorf("load proxy %d for CPA sync: %w", *account.ProxyID, err)
	}
	if proxy == nil {
		return "", nil
	}
	return strings.TrimSpace(proxy.URL()), nil
}

func persistGitWritableSnapshot(ctx context.Context, rootDir string, cfg Config, changedPaths []string, commitMessage string) error {
	repoDir := filepath.Clean(rootDir)
	if commitMessage == "" {
		commitMessage = "Sync CPA store"
	}

	addArgs := append([]string{"add", "-A", "--"}, changedPaths...)
	if err := runGit(ctx, repoDir, addArgs...); err != nil {
		return err
	}
	statusArgs := append([]string{"status", "--porcelain", "--"}, changedPaths...)
	statusOut, err := runGitOutput(ctx, repoDir, statusArgs...)
	if err != nil {
		return err
	}
	if strings.TrimSpace(statusOut) == "" {
		return nil
	}

	if err := runGit(ctx, repoDir, "config", "user.name", gitCommitUser(cfg)); err != nil {
		return err
	}
	if err := runGit(ctx, repoDir, "config", "user.email", gitCommitEmail(cfg)); err != nil {
		return err
	}
	if err := runGit(ctx, repoDir, "commit", "--no-gpg-sign", "-m", commitMessage); err != nil {
		return err
	}

	if strings.TrimSpace(cfg.GitBranch) != "" {
		return runGit(ctx, repoDir, "push", "origin", "HEAD:refs/heads/"+cfg.GitBranch)
	}
	return runGit(ctx, repoDir, "push", "origin", "HEAD")
}

func gitCommitUser(cfg Config) string {
	if user := strings.TrimSpace(cfg.GitUser); user != "" {
		return user
	}
	return "sub2api"
}

func gitCommitEmail(cfg Config) string {
	return sanitizeNameToken(gitCommitUser(cfg)) + "@local.invalid"
}

func runGit(ctx context.Context, repoDir string, args ...string) error {
	_, err := runGitCommand(ctx, repoDir, args...)
	return err
}

func runGitOutput(ctx context.Context, repoDir string, args ...string) (string, error) {
	return runGitCommand(ctx, repoDir, args...)
}

func runGitCommand(ctx context.Context, repoDir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repoDir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(output.String()))
	}
	return output.String(), nil
}

func ensureLegacyStoreLayout(rootDir string) error {
	authDir, err := resolveStorePath(rootDir, legacyAuthsDirName)
	if err != nil {
		return err
	}
	configPath, err := resolveStorePath(rootDir, legacyConfigRelativePath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		return fmt.Errorf("create CPA auth dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("create CPA config dir: %w", err)
	}
	return nil
}

func resolveStorePath(rootDir string, parts ...string) (string, error) {
	repoRoot := filepath.Clean(strings.TrimSpace(rootDir))
	if repoRoot == "" {
		return "", fmt.Errorf("empty CPA store root")
	}
	fullPath := repoRoot
	for _, part := range parts {
		cleanPart := filepath.Clean(strings.TrimSpace(part))
		if cleanPart == "." || cleanPart == "" {
			continue
		}
		if filepath.IsAbs(cleanPart) {
			return "", fmt.Errorf("absolute CPA store path not allowed: %s", cleanPart)
		}
		fullPath = filepath.Join(fullPath, cleanPart)
	}
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", fmt.Errorf("resolve CPA store root: %w", err)
	}
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("resolve CPA store path: %w", err)
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return "", fmt.Errorf("check CPA store path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("CPA store path escapes root: %s", strings.Join(parts, "/"))
	}
	return absPath, nil
}

func writeJSONIfChanged(path string, payload map[string]any) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("create CPA auth parent dir: %w", err)
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return false, fmt.Errorf("marshal CPA auth json: %w", err)
	}
	data = append(data, '\n')
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, data) {
		return false, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("read existing CPA auth json: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return false, fmt.Errorf("write CPA auth json: %w", err)
	}
	return true, nil
}

func deleteFileIfExists(path string) (bool, error) {
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("delete CPA store file: %w", err)
	}
	return true, nil
}

func updateLegacyConfigAPIKeys(path, key string, present bool) (bool, error) {
	root, err := loadLegacyConfigNode(path)
	if err != nil {
		return false, err
	}
	keysNode := ensureYAMLMappingValue(root, "api-keys", yaml.SequenceNode)
	existing := yamlSequenceStrings(keysNode)
	next := make([]string, 0, len(existing)+1)
	seen := make(map[string]struct{}, len(existing)+1)
	for _, item := range existing {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if trimmed == key && !present {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		next = append(next, trimmed)
	}
	if present {
		trimmed := strings.TrimSpace(key)
		if trimmed != "" {
			if _, ok := seen[trimmed]; !ok {
				next = append(next, trimmed)
			}
		}
	}
	sort.Strings(next)
	if slicesEqual(existing, next) {
		return false, nil
	}
	setYAMLSequenceStrings(keysNode, next)
	return writeYAMLNodeIfChanged(path, root)
}

func loadLegacyConfigNode(path string) (*yaml.Node, error) {
	root := &yaml.Node{Kind: yaml.DocumentNode}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			root.Content = []*yaml.Node{{Kind: yaml.MappingNode}}
			return root, nil
		}
		return nil, fmt.Errorf("read CPA config yaml: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		root.Content = []*yaml.Node{{Kind: yaml.MappingNode}}
		return root, nil
	}
	if err := yaml.Unmarshal(data, root); err != nil {
		return nil, fmt.Errorf("parse CPA config yaml: %w", err)
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		root.Content = []*yaml.Node{{Kind: yaml.MappingNode}}
	}
	return root, nil
}

func writeYAMLNodeIfChanged(path string, root *yaml.Node) (bool, error) {
	if root == nil {
		return false, fmt.Errorf("nil YAML root")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("create CPA config parent dir: %w", err)
	}
	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)
	if err := encoder.Encode(root); err != nil {
		_ = encoder.Close()
		return false, fmt.Errorf("encode CPA config yaml: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return false, fmt.Errorf("close CPA config yaml encoder: %w", err)
	}
	data := buffer.Bytes()
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, data) {
		return false, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("read existing CPA config yaml: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return false, fmt.Errorf("write CPA config yaml: %w", err)
	}
	return true, nil
}

func ensureYAMLMappingValue(root *yaml.Node, key string, kind yaml.Kind) *yaml.Node {
	if root.Kind != yaml.DocumentNode {
		root = &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		root.Content = []*yaml.Node{{Kind: yaml.MappingNode}}
	}
	mapping := root.Content[0]
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if strings.TrimSpace(mapping.Content[i].Value) == key {
			value := mapping.Content[i+1]
			if value.Kind != kind {
				value.Kind = kind
				value.Tag = ""
				value.Content = nil
				value.Value = ""
			}
			return value
		}
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	valueNode := &yaml.Node{Kind: kind}
	mapping.Content = append(mapping.Content, keyNode, valueNode)
	return valueNode
}

func yamlSequenceStrings(node *yaml.Node) []string {
	if node == nil || node.Kind != yaml.SequenceNode {
		return nil
	}
	out := make([]string, 0, len(node.Content))
	for _, child := range node.Content {
		trimmed := strings.TrimSpace(child.Value)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func setYAMLSequenceStrings(node *yaml.Node, values []string) {
	node.Kind = yaml.SequenceNode
	node.Tag = ""
	node.Value = ""
	node.Content = make([]*yaml.Node, 0, len(values))
	for _, value := range values {
		node.Content = append(node.Content, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Tag:   "!!str",
			Value: value,
		})
	}
}

func slicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func shouldSyncLegacyAPIKey(apiKey *service.APIKey, owner *service.User) bool {
	if apiKey == nil || owner == nil {
		return false
	}
	if strings.TrimSpace(apiKey.Key) == "" {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(owner.Email), legacyUserEmail)
}

func isCPACompatibleAccount(account *service.Account) bool {
	if account == nil {
		return false
	}
	switch account.Platform {
	case service.PlatformAnthropic, service.PlatformOpenAI, service.PlatformGemini, service.PlatformAntigravity:
	default:
		return false
	}
	switch account.Type {
	case service.AccountTypeOAuth, service.AccountTypeAPIKey, service.AccountTypeServiceAccount, service.AccountTypeSetupToken:
		return true
	default:
		return false
	}
}

func legacyAuthFileName(account *service.Account) string {
	if account == nil {
		return ""
	}
	if fileName := strings.TrimSpace(account.GetExtraString("legacy_cpa_file")); fileName != "" {
		return normalizeAuthFileName(fileName)
	}
	return fmt.Sprintf("sub2api-account-%d.json", account.ID)
}

func normalizeAuthFileName(fileName string) string {
	value := filepath.ToSlash(strings.TrimSpace(fileName))
	value = strings.TrimPrefix(value, "/")
	value = strings.TrimPrefix(value, legacyAuthsDirName+"/")
	if !strings.HasSuffix(strings.ToLower(value), ".json") {
		value += ".json"
	}
	return value
}

func determineLegacyProvider(account *service.Account) string {
	if account == nil {
		return ""
	}
	if provider := strings.TrimSpace(account.GetExtraString("legacy_cpa_provider")); provider != "" {
		return normalizeProvider(provider)
	}
	switch account.Platform {
	case service.PlatformAnthropic:
		return "claude"
	case service.PlatformOpenAI:
		if account.Type == service.AccountTypeAPIKey {
			return "openai"
		}
		return "codex"
	case service.PlatformGemini:
		if account.Type == service.AccountTypeServiceAccount {
			return "vertex"
		}
		oauthType := strings.ToLower(strings.TrimSpace(account.GetCredential("oauth_type")))
		if oauthType == "ai_studio" {
			return "aistudio"
		}
		if oauthType == "code_assist" {
			return "gemini"
		}
		if serviceAccount, ok := account.Credentials["service_account"].(map[string]any); ok && len(serviceAccount) > 0 {
			return "vertex"
		}
		return "gemini"
	case service.PlatformAntigravity:
		return "antigravity"
	default:
		return ""
	}
}

func baseLegacyAuthDocument(account *service.Account) map[string]any {
	if account == nil {
		return map[string]any{}
	}
	raw, ok := account.Extra["legacy_cpa_raw"].(map[string]any)
	if !ok || usesWrappedLegacyAuth(raw) {
		return map[string]any{}
	}
	doc, ok := cloneJSONValue(raw).(map[string]any)
	if !ok || doc == nil {
		return map[string]any{}
	}
	for _, key := range []string{"provider", "attributes", "metadata", "id", "status", "status_message", "unavailable", "quota"} {
		delete(doc, key)
	}
	return doc
}

func applyCredentialValue(dst map[string]any, credentials map[string]any, key string) {
	if dst == nil || credentials == nil {
		return
	}
	value, ok := credentials[key]
	if !ok {
		delete(dst, key)
		return
	}
	cloned := cloneJSONValue(value)
	if str, ok := cloned.(string); ok {
		trimmed := strings.TrimSpace(str)
		if trimmed == "" {
			delete(dst, key)
			return
		}
		dst[key] = trimmed
	} else if cloned != nil {
		dst[key] = cloned
	} else {
		delete(dst, key)
	}

	if tokenMap, ok := dst["token"].(map[string]any); ok {
		switch key {
		case "access_token", "refresh_token", "id_token", "client_id", "client_secret", "token_uri", "token_type":
			tokenMap[key] = cloneJSONValue(value)
		}
	}
}

func cloneJSONValue(value any) any {
	if value == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var cloned any
	if err := json.Unmarshal(data, &cloned); err != nil {
		return value
	}
	return cloned
}

func normalizedNotes(notes *string) string {
	if notes == nil {
		return ""
	}
	return strings.TrimSpace(*notes)
}
