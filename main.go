package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ELLIO-Technology/ellio_traefik_forward_auth/auth"
	"github.com/ELLIO-Technology/ellio_traefik_forward_auth/config"
	"github.com/ELLIO-Technology/ellio_traefik_forward_auth/edl"
	"github.com/ELLIO-Technology/ellio_traefik_forward_auth/logger"
	"github.com/ELLIO-Technology/ellio_traefik_forward_auth/logs"
	"github.com/ELLIO-Technology/ellio_traefik_forward_auth/metrics"
	"github.com/ELLIO-Technology/ellio_traefik_forward_auth/ipmatcher"
	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Build-time variables injected via ldflags
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

func main() {
	// Parse command-line flags
	versionFlag := flag.Bool("version", false, "Print version information and exit")
	flag.Parse()

	// Handle version flag
	if *versionFlag {
		fmt.Printf("ELLIO Traefik ForwardAuth\n")
		fmt.Printf("Version:    %s\n", Version)
		fmt.Printf("Git Commit: %s\n", GitCommit)
		fmt.Printf("Build Date: %s\n", BuildDate)
		fmt.Printf("Go Version: %s\n", "1.23")
		os.Exit(0)
	}
	// Initialize Sentry
	err := sentry.Init(sentry.ClientOptions{
		Dsn: "https://e4b93cc0954670220bdb84761e564355@o4505402528169984.ingest.us.sentry.io/4509940298612736",
		Environment: "production",
		TracesSampleRate: 0.1,
	})
	if err != nil {
		logger.Warn("Sentry initialization failed", "error", err)
	}
	defer sentry.Flush(2 * time.Second)

	cfg, err := config.Load()
	if err != nil {
		sentry.CaptureException(err)
		logger.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Log version information at startup
	logger.Info("Starting ELLIO Traefik ForwardAuth",
		"version", Version,
		"commit", GitCommit,
		"built", BuildDate,
	)

	// Set version info in auth package for health endpoint
	auth.SetVersionInfo(Version, GitCommit, BuildDate)

	logConfig(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize core components
	matcher := ipmatcher.New()
	updater := initEDL(ctx, cfg, matcher)
	authHandler := initAuthHandler(cfg, matcher, updater)

	// Start servers
	server := startMainServer(cfg, authHandler, auth.NewHealthHandler(updater))
	metricsServer := startMetricsServer(cfg)

	// Handle shutdown
	waitForShutdown(ctx, cancel, server, metricsServer, authHandler.logShipper, authHandler.metricsCollector)
}

func logConfig(cfg *config.Config) {
	logger.Info("Starting ForwardAuth server",
		"port", cfg.Port,
		"metrics_port", cfg.MetricsPort)
	
	if cfg.DeploymentEnabled {
		logger.Debug("EDL configuration",
			"url", cfg.EDLURL,
			"mode", cfg.EDLMode,
			"update_frequency", cfg.UpdateFrequency)
	} else {
		logger.Info("Deployment is disabled - allowing all traffic")
	}
}

func initEDL(ctx context.Context, cfg *config.Config, matcher *ipmatcher.Matcher) *edl.Updater {
	updater := edl.NewUpdater(cfg, matcher)

	if cfg.DeploymentEnabled {
		logger.Debug("Fetching initial EDL...")
		if err := updater.Start(ctx); err != nil {
			logger.Error("Failed to start EDL updater", "error", err)
			os.Exit(1)
		}

		// Initialize EDL metrics
		lastUpdate, _, updateCount, entryCount := updater.GetStatus()
		metrics.EDLEntries.Set(float64(entryCount))
		if !lastUpdate.IsZero() {
			metrics.EDLLastUpdateTimestamp.Set(float64(lastUpdate.Unix()))
		}
		for i := int64(0); i < updateCount; i++ {
			metrics.EDLUpdatesTotal.WithLabelValues("success").Inc()
		}
	} else {
		metrics.EDLEntries.Set(0)
	}

	return updater
}

type AuthHandlerWithDeps struct {
	*auth.Handler
	logShipper       *logs.LogShipper
	metricsCollector *logs.MetricsCollector
}

func initAuthHandler(cfg *config.Config, matcher *ipmatcher.Matcher, updater *edl.Updater) *AuthHandlerWithDeps {
	handler := auth.NewHandler(matcher, cfg.EDLMode, cfg.DeploymentEnabled)

	if cfg.IPHeaderOverride != "" {
		handler.SetIPHeaderOverride(cfg.IPHeaderOverride)
		logger.Debug("Using custom IP header", "header", cfg.IPHeaderOverride)
	}

	result := &AuthHandlerWithDeps{Handler: handler}

	// Initialize log shipping if configured
	if cfg.TokenManager.GetLogsURL() != "" {
		result.logShipper, result.metricsCollector = initLogShipping(cfg, handler)
	}

	return result
}

func initLogShipping(cfg *config.Config, handler *auth.Handler) (*logs.LogShipper, *logs.MetricsCollector) {
	logger.Debug("Initializing log shipping", "url", cfg.TokenManager.GetLogsURL())

	shipperConfig := &logs.LogShipperConfig{
		BatchSize:      cfg.LogBatchSize,
		FlushInterval:  cfg.LogFlushInterval,
		BucketCapacity: cfg.LeakyBucketCapacity,
		RefillRate:     cfg.LeakyBucketRefillRate,
		BufferSize:     cfg.LogBufferSize,
	}

	logShipper := logs.NewLogShipper(cfg.TokenManager, shipperConfig)
	logShipper.Start()

	handler.SetLogShipper(logShipper)
	handler.SetDeviceID(cfg.DeviceID)

	metricsCollector := logs.NewMetricsCollector(logShipper, nil, nil)
	metricsCollector.Start()

	logger.Debug("Log shipping initialized",
		"batch_size", cfg.LogBatchSize,
		"flush_interval", cfg.LogFlushInterval)

	return logShipper, metricsCollector
}

func startMainServer(cfg *config.Config, authHandler *AuthHandlerWithDeps, healthHandler *auth.HealthHandler) *http.Server {
	// Create Sentry handler
	sentryHandler := sentryhttp.New(sentryhttp.Options{
		Repanic: true,
	})

	mux := http.NewServeMux()

	// Serve static files (only from /static directory for security)
	fs := http.FileServer(http.Dir("/static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// Auth and health endpoints with Sentry middleware
	mux.Handle("/auth", sentryHandler.Handle(authHandler.Handler))
	mux.Handle("/", sentryHandler.Handle(authHandler.Handler))
	mux.HandleFunc("/health", healthHandler.Health)
	mux.HandleFunc("/ready", healthHandler.Ready)

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	go func() {
		logger.Info("Starting auth server", "port", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server error", "error", err)
			os.Exit(1)
		}
	}()

	return server
}

func startMetricsServer(cfg *config.Config) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	// Add pprof endpoints for profiling
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	mux.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
	mux.Handle("/debug/pprof/block", pprof.Handler("block"))

	server := &http.Server{
		Addr:              ":" + cfg.MetricsPort,
		Handler:           mux,
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	go func() {
		logger.Info("Starting metrics server", "port", cfg.MetricsPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Metrics server error", "error", err)
		}
	}()

	return server
}

func waitForShutdown(ctx context.Context, cancel context.CancelFunc, server, metricsServer *http.Server, logShipper *logs.LogShipper, metricsCollector *logs.MetricsCollector) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutting down servers...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Stop log shipper and flush remaining events
	if logShipper != nil {
		logger.Debug("Flushing log events...")
		if err := logShipper.Stop(); err != nil {
			logger.Error("Error stopping log shipper", "error", err)
		}
	}

	// Stop metrics collector
	if metricsCollector != nil {
		metricsCollector.Stop()
	}

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server shutdown error", "error", err)
	}

	if err := metricsServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("Metrics server shutdown error", "error", err)
	}

	cancel()
	logger.Info("Server stopped")
}
