package cpaimport

import (
	"fmt"
	"os"
	"strings"
)

// LoadConfigFromEnv reads CPA import bootstrap configuration from environment variables.
func LoadConfigFromEnv() (Config, error) {
	enabled := false
	if rawEnabled, exists := os.LookupEnv("CPA_IMPORT_ENABLED"); exists {
		enabled = parseBool(rawEnabled)
	}
	cfg := Config{
		SourceDir: strings.TrimSpace(os.Getenv("CPA_IMPORT_SOURCE_DIR")),
		GitURL:    strings.TrimSpace(os.Getenv("GITSTORE_GIT_URL")),
		GitUser:   strings.TrimSpace(os.Getenv("GITSTORE_GIT_USERNAME")),
		GitToken:  strings.TrimSpace(os.Getenv("GITSTORE_GIT_TOKEN")),
		GitBranch: strings.TrimSpace(os.Getenv("GITSTORE_GIT_BRANCH")),
	}
	if _, exists := os.LookupEnv("CPA_IMPORT_ENABLED"); !exists {
		enabled = cfg.SourceDir != "" || cfg.GitURL != ""
	}
	cfg.Enabled = enabled
	if !cfg.Enabled {
		return cfg, nil
	}
	if cfg.SourceDir == "" && cfg.GitURL == "" {
		return Config{}, fmt.Errorf("CPA_IMPORT_ENABLED=true but no source configured; set CPA_IMPORT_SOURCE_DIR or GITSTORE_GIT_URL")
	}
	return cfg, nil
}

func parseBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
