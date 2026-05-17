package cpaconvert

import "github.com/Wei-Shaw/sub2api/internal/transferdata"

// LegacyAuth represents one CPA auth JSON entry.
type LegacyAuth struct {
	ID         string            `json:"id"`
	Provider   string            `json:"provider"`
	Prefix     string            `json:"prefix,omitempty"`
	Label      string            `json:"label,omitempty"`
	Disabled   bool              `json:"disabled"`
	ProxyURL   string            `json:"proxy_url,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Metadata   map[string]any    `json:"metadata,omitempty"`
	FileName   string            `json:"-"`
	Raw        map[string]any    `json:"-"`
}

// ImportAccountSpec is the normalized intermediate representation before
// building sub2api import payloads.
type ImportAccountSpec struct {
	LegacyID    string
	Name        string
	Platform    string
	AccountType string
	Status      string
	Notes       *string
	ProxyURL    string
	Credentials map[string]any
	Extra       map[string]any
}

// PreservedAPIKey stores legacy CPA API keys for manual follow-up import.
type PreservedAPIKey struct {
	Name   string `json:"name"`
	Key    string `json:"key"`
	SHA256 string `json:"sha256"`
	Source string `json:"source"`
}

// SkippedAccount records legacy accounts that cannot be safely imported.
type SkippedAccount struct {
	FileName  string   `json:"file_name"`
	LegacyID  string   `json:"legacy_id,omitempty"`
	Name      string   `json:"name,omitempty"`
	Provider  string   `json:"provider,omitempty"`
	Reason    string   `json:"reason"`
	Warnings  []string `json:"warnings,omitempty"`
	ProxyURL  string   `json:"proxy_url,omitempty"`
	Disabled  bool     `json:"disabled,omitempty"`
	Suggested string   `json:"suggested,omitempty"`
}

// Summary aggregates conversion counts.
type Summary struct {
	AccountsSeen       int `json:"accounts_seen"`
	AccountsConverted  int `json:"accounts_converted"`
	AccountsSkipped    int `json:"accounts_skipped"`
	ProxiesGenerated   int `json:"proxies_generated"`
	APIKeysPreserved   int `json:"api_keys_preserved"`
	WarningsCount      int `json:"warnings_count"`
	DisabledSkipped    int `json:"disabled_skipped"`
	ServiceAcctSkipped int `json:"service_account_skipped"`
}

// Result contains all generated outputs.
type Result struct {
	DataPayload      transferdata.DataPayload `json:"data_payload"`
	PreservedAPIKeys []PreservedAPIKey        `json:"preserved_api_keys"`
	SkippedAccounts  []SkippedAccount         `json:"skipped_accounts"`
	Warnings         []string                 `json:"warnings"`
	Summary          Summary                  `json:"summary"`
}
