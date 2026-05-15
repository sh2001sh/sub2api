package cpaimport

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func loadSnapshot(ctx context.Context, cfg Config) (*snapshot, error) {
	if strings.TrimSpace(cfg.GitURL) != "" {
		return cloneGitSnapshot(ctx, cfg)
	}
	rootDir := strings.TrimSpace(cfg.SourceDir)
	if rootDir == "" {
		return nil, fmt.Errorf("empty CPA import source directory")
	}
	if _, err := os.Stat(rootDir); err != nil {
		return nil, fmt.Errorf("stat CPA import source directory: %w", err)
	}
	return &snapshot{
		rootDir: rootDir,
		cleanup: func() {},
	}, nil
}

func cloneGitSnapshot(ctx context.Context, cfg Config) (*snapshot, error) {
	tempDir, err := os.MkdirTemp("", "sub2api-cpa-import-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir for CPA import: %w", err)
	}
	repoDir := filepath.Join(tempDir, "repo")

	cloneURL, err := buildCloneURL(cfg)
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, err
	}

	args := []string{"clone", "--depth", "1"}
	if cfg.GitBranch != "" {
		args = append(args, "--branch", cfg.GitBranch)
	}
	args = append(args, cloneURL, repoDir)

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, fmt.Errorf("git clone CPA source failed: %w: %s", err, strings.TrimSpace(output.String()))
	}

	return &snapshot{
		rootDir: repoDir,
		cleanup: func() {
			_ = os.RemoveAll(tempDir)
		},
	}, nil
}

func buildCloneURL(cfg Config) (string, error) {
	if cfg.GitURL == "" {
		return "", fmt.Errorf("empty GITSTORE_GIT_URL")
	}
	if cfg.GitToken == "" {
		return cfg.GitURL, nil
	}
	parsed, err := url.Parse(cfg.GitURL)
	if err != nil {
		return "", fmt.Errorf("parse GITSTORE_GIT_URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("GITSTORE_GIT_URL must be http(s) when using token auth")
	}
	username := cfg.GitUser
	if username == "" {
		username = "git"
	}
	parsed.User = url.UserPassword(username, cfg.GitToken)
	return parsed.String(), nil
}
