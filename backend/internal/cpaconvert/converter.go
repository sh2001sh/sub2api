package cpaconvert

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/transferdata"
)

type proxyAccumulator struct {
	order []string
	items map[string]transferdata.DataProxy
}

// ConvertDir converts a CPA directory into original sub2api import artifacts.
func ConvertDir(rootDir string) (*Result, error) {
	auths, keys, err := loadLegacyData(rootDir)
	if err != nil {
		return nil, err
	}

	result := &Result{
		DataPayload: transferdata.DataPayload{
			Type:       "sub2api-data",
			Version:    1,
			ExportedAt: time.Now().UTC().Format(time.RFC3339),
			Proxies:    []transferdata.DataProxy{},
			Accounts:   []transferdata.DataAccount{},
		},
		PreservedAPIKeys: make([]PreservedAPIKey, 0, len(keys)),
		SkippedAccounts:  []SkippedAccount{},
		Warnings:         []string{},
		Summary: Summary{
			AccountsSeen: len(auths),
		},
	}

	proxies := proxyAccumulator{
		order: []string{},
		items: map[string]transferdata.DataProxy{},
	}

	for _, auth := range auths {
		spec, warnings, specErr := buildImportSpec(auth)
		if specErr != nil {
			result.addSkippedAccount(SkippedAccount{
				FileName: auth.FileName,
				LegacyID: auth.ID,
				Provider: normalizeProvider(auth.Provider),
				Reason:   specErr.Error(),
				Warnings: warnings,
				ProxyURL: auth.ProxyURL,
				Disabled: auth.Disabled,
				Suggested: "check provider mapping or fix malformed auth JSON before " +
					"retrying conversion",
			})
			continue
		}
		for _, warning := range warnings {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %s", auth.FileName, warning))
		}

		switch spec.AccountType {
		case service.AccountTypeServiceAccount:
			result.Summary.ServiceAcctSkipped++
			result.addSkippedAccount(SkippedAccount{
				FileName:  auth.FileName,
				LegacyID:  spec.LegacyID,
				Name:      spec.Name,
				Provider:  normalizeProvider(auth.Provider),
				Reason:    "original sub2api account-data import does not accept service_account accounts",
				Warnings:  warnings,
				ProxyURL:  spec.ProxyURL,
				Disabled:  auth.Disabled,
				Suggested: "create this Vertex/Gemini service account manually in the admin panel",
			})
			continue
		case service.AccountTypeBedrock:
			result.addSkippedAccount(SkippedAccount{
				FileName:  auth.FileName,
				LegacyID:  spec.LegacyID,
				Name:      spec.Name,
				Provider:  normalizeProvider(auth.Provider),
				Reason:    "bedrock account type is not produced by CPA conversion and cannot be auto-derived here",
				Warnings:  warnings,
				ProxyURL:  spec.ProxyURL,
				Disabled:  auth.Disabled,
				Suggested: "create this account manually if needed",
			})
			continue
		}

		if spec.Status == service.StatusDisabled {
			result.Summary.DisabledSkipped++
			result.addSkippedAccount(SkippedAccount{
				FileName:  auth.FileName,
				LegacyID:  spec.LegacyID,
				Name:      spec.Name,
				Provider:  normalizeProvider(auth.Provider),
				Reason:    "original sub2api account-data import cannot preserve disabled status safely",
				Warnings:  warnings,
				ProxyURL:  spec.ProxyURL,
				Disabled:  true,
				Suggested: "manually recreate this account and disable it after import if you still need it",
			})
			continue
		}

		proxyKey, proxyWarnings := proxies.add(spec.ProxyURL)
		result.Warnings = append(result.Warnings, proxyWarnings...)

		account := transferdata.DataAccount{
			Name:        spec.Name,
			Notes:       spec.Notes,
			Platform:    spec.Platform,
			Type:        spec.AccountType,
			Credentials: spec.Credentials,
			Extra:       spec.Extra,
			Concurrency: 1,
			Priority:    0,
			ExpiresAt:   extractExpiresAtUnix(spec.Credentials),
		}
		if proxyKey != "" {
			account.ProxyKey = &proxyKey
		}
		result.DataPayload.Accounts = append(result.DataPayload.Accounts, account)
		result.Summary.AccountsConverted++
	}

	for _, key := range proxies.order {
		result.DataPayload.Proxies = append(result.DataPayload.Proxies, proxies.items[key])
	}
	result.Summary.ProxiesGenerated = len(result.DataPayload.Proxies)

	for _, rawKey := range keys {
		name := fmt.Sprintf("legacy-cpa-key-%s", keySuffix(rawKey))
		result.PreservedAPIKeys = append(result.PreservedAPIKeys, PreservedAPIKey{
			Name:   name,
			Key:    rawKey,
			SHA256: hashAPIKey(rawKey),
			Source: "config/config.yaml:api-keys",
		})
	}
	result.Summary.APIKeysPreserved = len(result.PreservedAPIKeys)
	result.Summary.AccountsSkipped = len(result.SkippedAccounts)
	result.Summary.WarningsCount = len(result.Warnings)
	return result, nil
}

// WriteOutputs writes the conversion result into separate JSON artifacts.
func WriteOutputs(outDir string, result *Result) error {
	if result == nil {
		return fmt.Errorf("nil conversion result")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	files := map[string]any{
		"sub2api-import.json": result.DataPayload,
		"preserved-api-keys.json": map[string]any{
			"generated_at":       time.Now().UTC().Format(time.RFC3339),
			"preserved_api_keys": result.PreservedAPIKeys,
			"count":              len(result.PreservedAPIKeys),
		},
		"skipped-accounts.json": map[string]any{
			"generated_at":     time.Now().UTC().Format(time.RFC3339),
			"skipped_accounts": result.SkippedAccounts,
			"count":            len(result.SkippedAccounts),
		},
		"conversion-report.json": map[string]any{
			"generated_at": time.Now().UTC().Format(time.RFC3339),
			"summary":      result.Summary,
			"warnings":     result.Warnings,
		},
	}

	for name, payload := range files {
		path := filepath.Join(outDir, name)
		raw, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal %s: %w", name, err)
		}
		raw = append(raw, '\n')
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return nil
}

func (r *Result) addSkippedAccount(item SkippedAccount) {
	r.SkippedAccounts = append(r.SkippedAccounts, item)
	r.Warnings = append(r.Warnings, buildSkipWarning(item))
}

func buildSkipWarning(item SkippedAccount) string {
	label := item.FileName
	if strings.TrimSpace(label) == "" {
		label = item.LegacyID
	}
	return fmt.Sprintf("skip %s: %s", label, item.Reason)
}

func (p *proxyAccumulator) add(rawProxyURL string) (string, []string) {
	normalized, parsed, err := normalizeProxyURL(rawProxyURL)
	if err != nil {
		return "", []string{fmt.Sprintf("proxy %q preserved only in account extra: %v", rawProxyURL, err)}
	}
	if normalized == "" || parsed == nil {
		return "", nil
	}

	host := parsed.Hostname()
	portText := parsed.Port()
	if host == "" || portText == "" {
		return "", []string{fmt.Sprintf("proxy %q preserved only in account extra: missing host or port", rawProxyURL)}
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return "", []string{fmt.Sprintf("proxy %q preserved only in account extra: invalid port", rawProxyURL)}
	}
	username := ""
	password := ""
	if parsed.User != nil {
		username = parsed.User.Username()
		password, _ = parsed.User.Password()
	}
	key := buildProxyKey(parsed.Scheme, host, port, username, password)
	if _, exists := p.items[key]; exists {
		return key, nil
	}
	p.items[key] = transferdata.DataProxy{
		ProxyKey: key,
		Name:     defaultProxyName(host, port),
		Protocol: strings.ToLower(strings.TrimSpace(parsed.Scheme)),
		Host:     host,
		Port:     port,
		Username: username,
		Password: password,
		Status:   service.StatusActive,
	}
	p.order = append(p.order, key)
	return key, nil
}

func defaultProxyName(host string, port int) string {
	host = strings.TrimSpace(host)
	if host == "" || port <= 0 {
		return "imported-proxy"
	}
	return fmt.Sprintf("cpa-proxy-%s-%d", sanitizeNameToken(host), port)
}

func buildProxyKey(protocol, host string, port int, username, password string) string {
	return fmt.Sprintf("%s|%s|%d|%s|%s",
		strings.TrimSpace(protocol),
		strings.TrimSpace(host),
		port,
		strings.TrimSpace(username),
		strings.TrimSpace(password),
	)
}

func keySuffix(key string) string {
	value := strings.TrimSpace(key)
	if len(value) <= 6 {
		return value
	}
	return value[len(value)-6:]
}

func extractExpiresAtUnix(credentials map[string]any) *int64 {
	if len(credentials) == 0 {
		return nil
	}
	raw, ok := credentials["expires_at"]
	if !ok || raw == nil {
		return nil
	}
	text := strings.TrimSpace(fmt.Sprint(raw))
	if text == "" {
		return nil
	}
	if ts, err := strconv.ParseInt(text, 10, 64); err == nil {
		return &ts
	}
	if parsed, err := time.Parse(time.RFC3339, text); err == nil {
		value := parsed.Unix()
		return &value
	}
	if parsed, err := time.Parse(time.RFC3339Nano, text); err == nil {
		value := parsed.Unix()
		return &value
	}
	return nil
}
