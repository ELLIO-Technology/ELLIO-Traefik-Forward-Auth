package config

import (
	"context"
	"time"

	"github.com/ELLIO-Technology/ellio_traefik_forward_auth/api"
)

type Config struct {
	BootstrapToken    string
	EDLURL            string
	EDLMode           string
	UpdateFrequency   time.Duration
	Port              string
	MetricsPort       string
	LogLevel          string
	MaxRetryAttempts  int
	RetryDelay        time.Duration
	TokenManager      *api.TokenManager
	ConfigClient      *api.ConfigClient
	DeploymentEnabled bool
	// Log shipping configuration
	LogBatchSize          int
	LogFlushInterval      time.Duration
	LeakyBucketCapacity   int64
	LeakyBucketRefillRate int64
	LogBufferSize         int
	DeviceID              string
	// IP extraction configuration
	IPHeaderOverride string
}

// Load loads configuration and initializes services
// This maintains backward compatibility with existing code
func Load() (*Config, error) {
	cfg := LoadFromEnv()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := cfg.InitializeServices(ctx); err != nil {
		return nil, err
	}

	return cfg, nil
}
