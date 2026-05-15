package cpaimport

import "github.com/Wei-Shaw/sub2api/internal/service"

const (
	legacyUserEmail = "legacy-cpa@local.invalid"
	legacyUserName  = "legacy-cpa"
)

type Config struct {
	Enabled   bool
	SourceDir string
	GitURL    string
	GitUser   string
	GitToken  string
	GitBranch string
}

type BootstrapResult struct {
	Enabled         bool     `json:"enabled"`
	Source          string   `json:"source"`
	AccountsSeen    int      `json:"accounts_seen"`
	AccountsCreated int      `json:"accounts_created"`
	AccountsUpdated int      `json:"accounts_updated"`
	AccountsSkipped int      `json:"accounts_skipped"`
	KeysSeen        int      `json:"keys_seen"`
	KeysCreated     int      `json:"keys_created"`
	KeysSkipped     int      `json:"keys_skipped"`
	Warnings        []string `json:"warnings"`
}

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
	Checksum    string
}

type ImportMapping struct {
	LegacyType string
	LegacyID   string
	TargetKind string
	TargetID   int64
	Checksum   string
}

type snapshot struct {
	rootDir string
	cleanup func()
}

type proxyResolver struct {
	admin service.AdminService
	byURL map[string]int64
}
