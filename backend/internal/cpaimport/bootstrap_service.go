package cpaimport

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"log/slog"
)

type BootstrapService struct {
	stateRepo    *StateRepo
	userRepo     service.UserRepository
	adminService service.AdminService
	apiKeyRepo   service.APIKeyRepository
	apiKeySvc    *service.APIKeyService
}

// NewBootstrapService creates the CPA compatibility bootstrap service.
func NewBootstrapService(
	stateRepo *StateRepo,
	userRepo service.UserRepository,
	adminService service.AdminService,
	apiKeyRepo service.APIKeyRepository,
	apiKeySvc *service.APIKeyService,
) *BootstrapService {
	return &BootstrapService{
		stateRepo:    stateRepo,
		userRepo:     userRepo,
		adminService: adminService,
		apiKeyRepo:   apiKeyRepo,
		apiKeySvc:    apiKeySvc,
	}
}

// Run executes the startup CPA compatibility import if enabled.
func (s *BootstrapService) Run(ctx context.Context) (*BootstrapResult, error) {
	cfg, err := LoadConfigFromEnv()
	if err != nil {
		return nil, err
	}
	result := &BootstrapResult{Enabled: cfg.Enabled, Warnings: []string{}}
	if !cfg.Enabled {
		return result, nil
	}

	snap, err := loadSnapshot(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer snap.cleanup()

	auths, keys, err := loadLegacyData(snap.rootDir)
	if err != nil {
		return nil, err
	}
	result.Source = snap.rootDir
	result.AccountsSeen = len(auths)
	result.KeysSeen = len(keys)

	runID, err := s.stateRepo.BeginRun(ctx, result.Source)
	if err != nil {
		return nil, err
	}

	runErr := s.runImport(ctx, auths, keys, result)
	status := "success"
	if runErr != nil {
		status = "failed"
	}
	if finishErr := s.stateRepo.FinishRun(ctx, runID, status, result, runErr); finishErr != nil {
		if runErr != nil {
			return nil, fmt.Errorf("%w; also failed to persist run status: %v", runErr, finishErr)
		}
		return nil, finishErr
	}
	if runErr != nil {
		return nil, runErr
	}
	return result, nil
}

func (s *BootstrapService) runImport(ctx context.Context, auths []LegacyAuth, keys []string, result *BootstrapResult) error {
	user, err := s.ensureLegacyUser(ctx)
	if err != nil {
		return err
	}

	proxies, err := s.newProxyResolver(ctx)
	if err != nil {
		return err
	}

	for _, auth := range auths {
		spec, warnings, specErr := buildImportSpec(auth)
		for _, warning := range warnings {
			result.Warnings = append(result.Warnings, fmt.Sprintf("auth %s: %s", auth.FileName, warning))
		}
		if specErr != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("skip auth %s: %v", auth.FileName, specErr))
			continue
		}
		if err := s.upsertAccount(ctx, spec, proxies, result); err != nil {
			return fmt.Errorf("import account %s: %w", spec.LegacyID, err)
		}
	}

	for _, rawKey := range keys {
		if err := s.importAPIKey(ctx, user.ID, rawKey, result); err != nil {
			return fmt.Errorf("import api key: %w", err)
		}
	}
	return nil
}

func (s *BootstrapService) ensureLegacyUser(ctx context.Context) (*service.User, error) {
	user, err := s.userRepo.GetByEmail(ctx, legacyUserEmail)
	if err == nil && user != nil {
		return user, nil
	}
	input := &service.CreateUserInput{
		Email:       legacyUserEmail,
		Password:    "cpa-import-bootstrap-disabled-login",
		Username:    legacyUserName,
		Notes:       "Synthetic compatibility user for imported CPA data",
		Concurrency: 100,
	}
	created, err := s.adminService.CreateUser(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("create legacy CPA user: %w", err)
	}
	return created, nil
}

func (s *BootstrapService) upsertAccount(ctx context.Context, spec *ImportAccountSpec, proxies *proxyResolver, result *BootstrapResult) error {
	var proxyID *int64
	if spec.ProxyURL != "" {
		resolvedProxyID, warning, err := proxies.resolve(ctx, spec.ProxyURL)
		if warning != "" {
			result.Warnings = append(result.Warnings, fmt.Sprintf("auth %s proxy: %s", spec.LegacyID, warning))
		}
		if err != nil {
			return err
		}
		proxyID = resolvedProxyID
	}

	mapping, err := s.stateRepo.GetMapping(ctx, "account", spec.LegacyID, "account")
	if err != nil {
		return err
	}
	if mapping != nil && mapping.Checksum == spec.Checksum {
		result.AccountsSkipped++
		return nil
	}

	if mapping != nil {
		account, err := s.adminService.GetAccount(ctx, mapping.TargetID)
		if err == nil && account != nil {
			if account.Platform != spec.Platform {
				return fmt.Errorf("existing account %d platform mismatch: have %s, want %s", account.ID, account.Platform, spec.Platform)
			}
			update := &service.UpdateAccountInput{
				Name:        spec.Name,
				Type:        spec.AccountType,
				Notes:       spec.Notes,
				Credentials: spec.Credentials,
				Extra:       spec.Extra,
				Status:      spec.Status,
				ProxyID:     updateProxyIDValue(proxyID),
			}
			if _, err := s.adminService.UpdateAccount(ctx, account.ID, update); err != nil {
				return fmt.Errorf("update account %d: %w", account.ID, err)
			}
			if err := s.stateRepo.UpsertMapping(ctx, ImportMapping{
				LegacyType: "account",
				LegacyID:   spec.LegacyID,
				TargetKind: "account",
				TargetID:   account.ID,
				Checksum:   spec.Checksum,
			}); err != nil {
				return err
			}
			result.AccountsUpdated++
			return nil
		}
	}

	create := &service.CreateAccountInput{
		Name:                 spec.Name,
		Notes:                spec.Notes,
		Platform:             spec.Platform,
		Type:                 spec.AccountType,
		Credentials:          spec.Credentials,
		Extra:                spec.Extra,
		ProxyID:              proxyID,
		Concurrency:          1,
		Priority:             0,
		SkipDefaultGroupBind: true,
	}
	account, err := s.adminService.CreateAccount(ctx, create)
	if err != nil {
		return fmt.Errorf("create account: %w", err)
	}
	if spec.Status == service.StatusDisabled {
		update := &service.UpdateAccountInput{
			Status:  service.StatusDisabled,
			ProxyID: updateProxyIDValue(proxyID),
		}
		if _, err := s.adminService.UpdateAccount(ctx, account.ID, update); err != nil {
			return fmt.Errorf("disable imported account %d: %w", account.ID, err)
		}
	}
	if err := s.stateRepo.UpsertMapping(ctx, ImportMapping{
		LegacyType: "account",
		LegacyID:   spec.LegacyID,
		TargetKind: "account",
		TargetID:   account.ID,
		Checksum:   spec.Checksum,
	}); err != nil {
		return err
	}
	result.AccountsCreated++
	return nil
}

func (s *BootstrapService) importAPIKey(ctx context.Context, userID int64, rawKey string, result *BootstrapResult) error {
	key := strings.TrimSpace(rawKey)
	if key == "" {
		result.KeysSkipped++
		return nil
	}
	hash := hashAPIKey(key)
	mapping, err := s.stateRepo.GetMapping(ctx, "api_key", hash, "api_key")
	if err != nil {
		return err
	}
	if mapping != nil {
		result.KeysSkipped++
		return nil
	}

	exists, err := s.apiKeyRepo.ExistsByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("check existing key: %w", err)
	}
	if exists {
		existing, err := s.apiKeyRepo.GetByKey(ctx, key)
		if err != nil {
			return fmt.Errorf("load existing key: %w", err)
		}
		if err := s.stateRepo.UpsertMapping(ctx, ImportMapping{
			LegacyType: "api_key",
			LegacyID:   hash,
			TargetKind: "api_key",
			TargetID:   existing.ID,
			Checksum:   hash,
		}); err != nil {
			return err
		}
		result.KeysSkipped++
		return nil
	}

	name := fmt.Sprintf("legacy-cpa-key-%s", keySuffix(key))
	created, err := s.apiKeySvc.Create(ctx, userID, service.CreateAPIKeyRequest{
		Name:      name,
		CustomKey: &key,
	})
	if err != nil {
		return fmt.Errorf("create legacy key %s: %w", name, err)
	}
	if err := s.stateRepo.UpsertMapping(ctx, ImportMapping{
		LegacyType: "api_key",
		LegacyID:   hash,
		TargetKind: "api_key",
		TargetID:   created.ID,
		Checksum:   hash,
	}); err != nil {
		return err
	}
	result.KeysCreated++
	return nil
}

func updateProxyIDValue(proxyID *int64) *int64 {
	if proxyID != nil {
		return proxyID
	}
	zero := int64(0)
	return &zero
}

func keySuffix(key string) string {
	value := strings.TrimSpace(key)
	if len(value) <= 6 {
		return value
	}
	return value[len(value)-6:]
}

func (s *BootstrapService) newProxyResolver(ctx context.Context) (*proxyResolver, error) {
	proxies, err := s.adminService.GetAllProxies(ctx)
	if err != nil {
		return nil, fmt.Errorf("list existing proxies: %w", err)
	}
	resolver := &proxyResolver{
		admin: s.adminService,
		byURL: make(map[string]int64, len(proxies)),
	}
	for _, proxy := range proxies {
		resolver.byURL[normalizeProxyKey(proxy.Protocol, proxy.Host, proxy.Port, proxy.Username, proxy.Password)] = proxy.ID
	}
	return resolver, nil
}

func (r *proxyResolver) resolve(ctx context.Context, rawURL string) (*int64, string, error) {
	normalized, parsed, err := normalizeProxyURL(rawURL)
	if err != nil {
		return nil, fmt.Sprintf("invalid proxy URL %q preserved only in extra", rawURL), nil
	}
	if normalized == "" || parsed == nil {
		return nil, "", nil
	}
	host := parsed.Hostname()
	portValue := parsed.Port()
	if host == "" || portValue == "" {
		return nil, fmt.Sprintf("proxy URL %q missing host or port; preserved only in extra", rawURL), nil
	}
	port, err := strconv.Atoi(portValue)
	if err != nil {
		return nil, fmt.Sprintf("proxy URL %q uses unsupported port %q; preserved only in extra", rawURL, portValue), nil
	}
	username := ""
	password := ""
	if parsed.User != nil {
		username = parsed.User.Username()
		password, _ = parsed.User.Password()
	}
	cacheKey := normalizeProxyKey(parsed.Scheme, host, port, username, password)
	if proxyID, ok := r.byURL[cacheKey]; ok {
		return &proxyID, "", nil
	}
	created, err := r.admin.CreateProxy(ctx, &service.CreateProxyInput{
		Name:     fmt.Sprintf("cpa-import-%s-%d", host, port),
		Protocol: parsed.Scheme,
		Host:     host,
		Port:     port,
		Username: username,
		Password: password,
	})
	if err != nil {
		return nil, "", fmt.Errorf("create proxy from legacy auth: %w", err)
	}
	r.byURL[cacheKey] = created.ID
	return &created.ID, "", nil
}

func normalizeProxyKey(protocol, host string, port int, username, password string) string {
	return fmt.Sprintf("%s://%s:%s@%s:%d", strings.ToLower(strings.TrimSpace(protocol)), username, password, strings.ToLower(strings.TrimSpace(host)), port)
}

func init() {
	slog.Debug("cpaimport package loaded")
}
