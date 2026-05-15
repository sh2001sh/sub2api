package setup

import (
	"os"
	"strings"
	"testing"
)

func TestDecideAdminBootstrap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		totalUsers int64
		adminUsers int64
		should     bool
		reason     string
	}{
		{
			name:       "empty database should create admin",
			totalUsers: 0,
			adminUsers: 0,
			should:     true,
			reason:     adminBootstrapReasonEmptyDatabase,
		},
		{
			name:       "admin exists should skip",
			totalUsers: 10,
			adminUsers: 1,
			should:     false,
			reason:     adminBootstrapReasonAdminExists,
		},
		{
			name:       "users exist without admin should skip",
			totalUsers: 5,
			adminUsers: 0,
			should:     false,
			reason:     adminBootstrapReasonUsersExistWithoutAdmin,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := decideAdminBootstrap(tc.totalUsers, tc.adminUsers)
			if got.shouldCreate != tc.should {
				t.Fatalf("shouldCreate=%v, want %v", got.shouldCreate, tc.should)
			}
			if got.reason != tc.reason {
				t.Fatalf("reason=%q, want %q", got.reason, tc.reason)
			}
		})
	}
}

func TestSetupDefaultAdminConcurrency(t *testing.T) {
	t.Run("simple mode admin uses higher concurrency", func(t *testing.T) {
		t.Setenv("RUN_MODE", "simple")
		if got := setupDefaultAdminConcurrency(); got != simpleModeAdminConcurrency {
			t.Fatalf("setupDefaultAdminConcurrency()=%d, want %d", got, simpleModeAdminConcurrency)
		}
	})

	t.Run("standard mode keeps existing default", func(t *testing.T) {
		t.Setenv("RUN_MODE", "standard")
		if got := setupDefaultAdminConcurrency(); got != defaultUserConcurrency {
			t.Fatalf("setupDefaultAdminConcurrency()=%d, want %d", got, defaultUserConcurrency)
		}
	})
}

func TestWriteConfigFileKeepsDefaultUserConcurrency(t *testing.T) {
	t.Setenv("RUN_MODE", "simple")
	t.Setenv("DATA_DIR", t.TempDir())

	if err := writeConfigFile(&SetupConfig{}); err != nil {
		t.Fatalf("writeConfigFile() error = %v", err)
	}

	data, err := os.ReadFile(GetConfigFilePath())
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if !strings.Contains(string(data), "user_concurrency: 5") {
		t.Fatalf("config missing default user concurrency, got:\n%s", string(data))
	}
}

func TestApplyConnectionURLEnvOverrides(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://dbuser:dbpass@db.example.com:6543/appdb?sslmode=require")
	t.Setenv("REDIS_URL", "rediss://:redispass@redis.example.com:6380/2")
	t.Setenv("DATABASE_HOST", "override-db.example.com")
	t.Setenv("REDIS_DB", "5")

	cfg := &SetupConfig{}
	if err := applyConnectionURLEnvOverrides(cfg); err != nil {
		t.Fatalf("applyConnectionURLEnvOverrides() error = %v", err)
	}

	if cfg.Database.Host != "override-db.example.com" {
		t.Fatalf("database host = %q, want %q", cfg.Database.Host, "override-db.example.com")
	}
	if cfg.Database.Port != 6543 {
		t.Fatalf("database port = %d, want 6543", cfg.Database.Port)
	}
	if cfg.Database.User != "dbuser" {
		t.Fatalf("database user = %q, want %q", cfg.Database.User, "dbuser")
	}
	if cfg.Database.Password != "dbpass" {
		t.Fatalf("database password = %q, want %q", cfg.Database.Password, "dbpass")
	}
	if cfg.Database.DBName != "appdb" {
		t.Fatalf("database name = %q, want %q", cfg.Database.DBName, "appdb")
	}
	if cfg.Database.SSLMode != "require" {
		t.Fatalf("database sslmode = %q, want %q", cfg.Database.SSLMode, "require")
	}

	if cfg.Redis.Host != "redis.example.com" {
		t.Fatalf("redis host = %q, want %q", cfg.Redis.Host, "redis.example.com")
	}
	if cfg.Redis.Port != 6380 {
		t.Fatalf("redis port = %d, want 6380", cfg.Redis.Port)
	}
	if cfg.Redis.Password != "redispass" {
		t.Fatalf("redis password = %q, want %q", cfg.Redis.Password, "redispass")
	}
	if cfg.Redis.DB != 5 {
		t.Fatalf("redis db = %d, want 5", cfg.Redis.DB)
	}
	if !cfg.Redis.EnableTLS {
		t.Fatalf("redis enable_tls = false, want true")
	}
}
