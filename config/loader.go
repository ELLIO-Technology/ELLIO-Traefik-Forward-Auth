package config

import (
	"context"
	"errors"
	"time"

	"github.com/ELLIO-Technology/ellio_traefik_forward_auth/api"
	"github.com/ELLIO-Technology/ellio_traefik_forward_auth/utils"
	"github.com/golang-jwt/jwt/v5"
	"github.com/keygen-sh/machineid"
)

// LoadFromEnv loads configuration from environment variables only
func LoadFromEnv() *Config {
	return &Config{
		BootstrapToken:        utils.GetEnv("ELLIO_BOOTSTRAP", ""),
		Port:                  utils.GetEnv("PORT", "8080"),
		MetricsPort:           utils.GetEnv("METRICS_PORT", "9090"),
		LogLevel:              utils.GetEnv("LOG_LEVEL", "info"),
		MaxRetryAttempts:      utils.GetEnvAsInt("MAX_RETRY_ATTEMPTS", 3),
		LogBatchSize:          utils.GetEnvAsInt("LOG_BATCH_SIZE", 100),
		LeakyBucketCapacity:   utils.GetEnvAsInt64("LEAKY_BUCKET_CAPACITY", 1000),
		LeakyBucketRefillRate: utils.GetEnvAsInt64("LEAKY_BUCKET_REFILL_RATE", 100),
		LogBufferSize:         utils.GetEnvAsInt("LOG_BUFFER_SIZE", 10000),
		IPHeaderOverride:      utils.GetEnv("IP_HEADER_OVERRIDE", ""),
		RetryDelay:            utils.GetEnvAsDuration("RETRY_DELAY", 30*time.Second),
		LogFlushInterval:      utils.GetEnvAsDuration("LOG_FLUSH_INTERVAL", 10*time.Second),
	}
}

// InitializeServices initializes external services and fetches EDL configuration
func (cfg *Config) InitializeServices(ctx context.Context) error {
	if cfg.BootstrapToken == "" {
		return errors.New("ELLIO_BOOTSTRAP token is required")
	}

	// Parse bootstrap token to get deployment ID
	if err := cfg.parseBootstrapToken(); err != nil {
		return err
	}

	// Initialize token manager
	cfg.TokenManager = api.NewTokenManager(cfg.BootstrapToken)

	// Bootstrap and get initial token
	if err := cfg.TokenManager.Initialize(ctx); err != nil {
		if api.IsPermanentError(err) {
			cfg.setDeploymentDisabled()
			return nil
		}
		return errors.New("failed to bootstrap: " + err.Error())
	}

	// Start token refresh loop
	cfg.TokenManager.StartRefreshLoop(context.Background())

	// Create config client and fetch EDL configuration
	cfg.ConfigClient = api.NewConfigClient(cfg.TokenManager)

	edlConfig, err := cfg.ConfigClient.GetEDLConfig(ctx)
	if err != nil {
		if api.IsPermanentError(err) {
			cfg.setDeploymentDisabled()
			return nil
		}
		return errors.New("failed to fetch EDL config: " + err.Error())
	}

	// Apply EDL configuration
	cfg.applyEDLConfig(edlConfig)

	return nil
}

func (cfg *Config) parseBootstrapToken() error {
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	var claims api.BootstrapClaims
	_, _, _ = parser.ParseUnverified(cfg.BootstrapToken, &claims)

	// Get protected machine ID using deployment ID as app key
	machineID, err := machineid.ProtectedID(claims.DeploymentID)
	if err != nil {
		machineID = "unknown"
	}
	cfg.DeviceID = machineID

	return nil
}

func (cfg *Config) setDeploymentDisabled() {
	cfg.DeploymentEnabled = false
	cfg.EDLMode = "disabled"
	cfg.UpdateFrequency = 1 * time.Hour
	cfg.EDLURL = ""
}

func (cfg *Config) applyEDLConfig(edlConfig *api.EDLConfig) {
	if !edlConfig.Enabled {
		cfg.setDeploymentDisabled()
		return
	}

	cfg.DeploymentEnabled = true

	// Map purpose to EDL mode
	switch edlConfig.Purpose {
	case "allowlist":
		cfg.EDLMode = "allowlist"
	case "blocklist", "other", "others":
		cfg.EDLMode = "blocklist"
	default:
		cfg.EDLMode = "blocklist"
	}

	cfg.UpdateFrequency = time.Duration(edlConfig.UpdateFrequencySeconds) * time.Second
	if cfg.UpdateFrequency <= 0 {
		cfg.UpdateFrequency = 5 * time.Minute
	}

	if len(edlConfig.URLs.Combined) > 0 {
		cfg.EDLURL = edlConfig.URLs.Combined[0]
	}
}
