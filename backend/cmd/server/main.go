package main

//go:generate go run github.com/google/wire/cmd/wire

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/cpaimport"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/setup"
	"github.com/Wei-Shaw/sub2api/internal/web"

	"github.com/gin-gonic/gin"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

//go:embed VERSION
var embeddedVersion string

// Build-time variables (can be set by ldflags).
var (
	Version   = ""
	Commit    = "unknown"
	Date      = "unknown"
	BuildType = "source" // "source" for manual builds, "release" for CI builds.
)

func init() {
	if strings.TrimSpace(Version) != "" {
		return
	}

	Version = strings.TrimSpace(embeddedVersion)
	if Version == "" {
		Version = "0.0.0-dev"
	}
}

func main() {
	logger.InitBootstrap()
	defer logger.Sync()

	setupMode := flag.Bool("setup", false, "Run setup wizard in CLI mode")
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion {
		log.Printf("Sub2API %s (commit: %s, built: %s)\n", Version, Commit, Date)
		return
	}

	if err := setup.EnsureEmbeddedRedisFromEnv(); err != nil {
		log.Fatalf("Embedded Redis bootstrap failed: %v", err)
	}

	if *setupMode {
		if err := setup.RunCLI(); err != nil {
			log.Fatalf("Setup failed: %v", err)
		}
		return
	}

	var bootstrapHealthServer *http.Server

	if setup.NeedsSetup() {
		if setup.AutoSetupEnabled() {
			log.Println("Auto setup mode enabled...")
			bootstrapHealthServer = startBootstrapHealthServer(config.GetServerAddress())
			if err := setup.AutoSetupFromEnv(); err != nil {
				shutdownBootstrapHealthServer(bootstrapHealthServer)
				log.Fatalf("Auto setup failed: %v", err)
			}
		} else {
			log.Println("First run detected, starting setup wizard...")
			runSetupServer()
			return
		}
	}

	runMainServer(bootstrapHealthServer)
}

func runSetupServer() {
	r := gin.New()
	r.Use(middleware.Recovery())
	r.Use(middleware.CORS(config.CORSConfig{}))
	r.Use(middleware.SecurityHeaders(config.CSPConfig{Enabled: true, Policy: config.DefaultCSPPolicy}, nil))

	setup.RegisterRoutes(r)

	if web.HasEmbeddedFrontend() {
		r.Use(web.ServeEmbeddedFrontend())
	}

	addr := config.GetServerAddress()
	log.Printf("Setup wizard available at http://%s", addr)
	log.Println("Complete the setup wizard to configure Sub2API")

	server := &http.Server{
		Addr:              addr,
		Handler:           h2c.NewHandler(r, &http2.Server{}),
		ReadHeaderTimeout: 30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("Failed to start setup server: %v", err)
	}
}

func runMainServer(bootstrapHealthServer *http.Server) {
	cfg, err := config.LoadForBootstrap()
	if err != nil {
		shutdownBootstrapHealthServer(bootstrapHealthServer)
		log.Fatalf("Failed to load config: %v", err)
	}
	if err := logger.Init(logger.OptionsFromConfig(cfg.Log)); err != nil {
		shutdownBootstrapHealthServer(bootstrapHealthServer)
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	if cfg.RunMode == config.RunModeSimple {
		log.Println("SIMPLE mode is enabled; billing and quota checks are disabled")
	}

	buildInfo := handler.BuildInfo{
		Version:   Version,
		BuildType: BuildType,
	}

	app, err := initializeApplication(buildInfo)
	if err != nil {
		shutdownBootstrapHealthServer(bootstrapHealthServer)
		log.Fatalf("Failed to initialize application: %v", err)
	}
	defer app.Cleanup()

	shutdownBootstrapHealthServer(bootstrapHealthServer)

	go func() {
		if err := app.Server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	log.Printf("Server started on %s", app.Server.Addr)
	startCPAImportBootstrap(app.CPAImportBootstrap)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := app.Server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}

func startBootstrapHealthServer(addr string) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"bootstrapping"}`))
	})
	mux.HandleFunc("/setup/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"needs_setup":true,"step":"auto_setup_running"}}`))
	})

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Failed to start bootstrap health server: %v", err)
		}
	}()

	log.Printf("Bootstrap health server listening on http://%s", addr)
	return server
}

func shutdownBootstrapHealthServer(server *http.Server) {
	if server == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("Failed to stop bootstrap health server: %v", err)
	}
}

func startCPAImportBootstrap(bootstrap *cpaimport.BootstrapService) {
	if bootstrap == nil {
		return
	}

	timeout := cpaImportBootstrapTimeout()
	go func() {
		log.Printf("Starting CPA import bootstrap in background (timeout=%s)", timeout)

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		result, err := bootstrap.Run(ctx)
		if err != nil {
			log.Printf("CPA import bootstrap failed: %v", err)
			return
		}
		if result == nil || !result.Enabled {
			log.Printf("CPA import bootstrap skipped")
			return
		}

		log.Printf(
			"CPA import bootstrap completed: source=%s accounts_seen=%d accounts_created=%d accounts_updated=%d accounts_skipped=%d keys_seen=%d keys_created=%d keys_skipped=%d warnings=%d",
			result.Source,
			result.AccountsSeen,
			result.AccountsCreated,
			result.AccountsUpdated,
			result.AccountsSkipped,
			result.KeysSeen,
			result.KeysCreated,
			result.KeysSkipped,
			len(result.Warnings),
		)
	}()
}

func cpaImportBootstrapTimeout() time.Duration {
	const defaultTimeout = 30 * time.Minute

	raw := strings.TrimSpace(os.Getenv("CPA_IMPORT_BOOTSTRAP_TIMEOUT_SECONDS"))
	if raw == "" {
		return defaultTimeout
	}

	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		log.Printf("Invalid CPA_IMPORT_BOOTSTRAP_TIMEOUT_SECONDS=%q, using default %s", raw, defaultTimeout)
		return defaultTimeout
	}
	return time.Duration(seconds) * time.Second
}
