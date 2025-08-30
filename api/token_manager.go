package api

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ELLIO-Technology/ellio_traefik_forward_auth/logger"
	"github.com/getsentry/sentry-go"
)

type TokenManager struct {
	bootstrapClient *BootstrapClient
	bootstrapToken  string

	mu                sync.RWMutex
	currentToken      string
	tokenExpiry       time.Time
	configURL         string
	logsURL           string
	deploymentDeleted bool

	refreshInterval time.Duration
	stopCh          chan struct{}
}

func NewTokenManager(bootstrapToken string) *TokenManager {
	return &TokenManager{
		bootstrapClient: NewBootstrapClient(),
		bootstrapToken:  bootstrapToken,
		refreshInterval: 5 * time.Minute,
		stopCh:          make(chan struct{}),
	}
}

func (tm *TokenManager) Initialize(ctx context.Context) error {
	// Perform initial bootstrap
	resp, err := tm.bootstrapClient.Bootstrap(ctx, tm.bootstrapToken)
	if err != nil {
		// Check if it's a permanent error (410)
		if IsPermanentError(err) {
			tm.mu.Lock()
			tm.deploymentDeleted = true
			tm.mu.Unlock()
			logger.Warn("Deployment has been permanently deleted (410). Switching to allow-all mode")
		}
		return errors.New("initial bootstrap failed: " + err.Error())
	}

	tm.mu.Lock()
	tm.currentToken = resp.AccessToken
	tm.tokenExpiry = time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second)
	tm.configURL = resp.ConfigURL
	tm.logsURL = resp.LogsURL
	tm.mu.Unlock()

	logger.Info("Bootstrap successful",
		"expires_in", resp.ExpiresIn,
		"config_url", resp.ConfigURL)

	return nil
}

func (tm *TokenManager) StartRefreshLoop(ctx context.Context) {
	go func() {
		// Don't start refresh loop if deployment is already deleted
		tm.mu.RLock()
		if tm.deploymentDeleted {
			tm.mu.RUnlock()
			logger.Debug("Not starting token refresh loop - deployment is deleted")
			return
		}
		tm.mu.RUnlock()

		// Calculate when to refresh (80% of token lifetime)
		refreshTimer := time.NewTimer(tm.calculateRefreshInterval())
		defer refreshTimer.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-tm.stopCh:
				return
			case <-refreshTimer.C:
				// Check if deployment was deleted
				tm.mu.RLock()
				deleted := tm.deploymentDeleted
				tm.mu.RUnlock()

				if deleted {
					logger.Debug("Stopping token refresh loop - deployment has been deleted")
					return
				}

				if err := tm.refresh(ctx); err != nil {
					logger.Error("Token refresh failed", "error", err)
					sentry.CaptureException(err)
					// Retry after a short delay
					refreshTimer.Reset(30 * time.Second)
				} else {
					refreshTimer.Reset(tm.calculateRefreshInterval())
				}
			}
		}
	}()
}

func (tm *TokenManager) calculateRefreshInterval() time.Duration {
	tm.mu.RLock()
	expiry := tm.tokenExpiry
	tm.mu.RUnlock()

	// Refresh at 80% of token lifetime
	timeUntilExpiry := time.Until(expiry)
	refreshAt := time.Duration(float64(timeUntilExpiry) * 0.8)

	// Minimum refresh interval of 30 seconds
	if refreshAt < 30*time.Second {
		refreshAt = 30 * time.Second
	}

	return refreshAt
}

func (tm *TokenManager) refresh(ctx context.Context) error {
	resp, err := tm.bootstrapClient.Bootstrap(ctx, tm.bootstrapToken)
	if err != nil {
		// Check if it's a permanent error (410)
		if IsPermanentError(err) {
			tm.mu.Lock()
			tm.deploymentDeleted = true
			tm.mu.Unlock()
			logger.Warn("Deployment has been permanently deleted (410) during refresh. Stopping refresh loop")
			return err
		}
		return errors.New("token refresh failed: " + err.Error())
	}

	tm.mu.Lock()
	tm.currentToken = resp.AccessToken
	tm.tokenExpiry = time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second)
	tm.configURL = resp.ConfigURL
	tm.logsURL = resp.LogsURL
	tm.mu.Unlock()

	logger.Debug("Token refreshed successfully",
		"expires_in", resp.ExpiresIn)

	return nil
}

func (tm *TokenManager) GetToken() string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.currentToken
}

func (tm *TokenManager) GetConfigURL() string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.configURL
}

func (tm *TokenManager) GetLogsURL() string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.logsURL
}

func (tm *TokenManager) IsDeploymentDeleted() bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.deploymentDeleted
}

func (tm *TokenManager) Stop() {
	close(tm.stopCh)
}

// GetTokenWithMinValidity returns a token that's valid for at least minValidity duration
// If current token doesn't have enough validity, it triggers a refresh
func (tm *TokenManager) GetTokenWithMinValidity(minValidity time.Duration) (string, error) {
	tm.mu.RLock()
	timeRemaining := time.Until(tm.tokenExpiry)
	token := tm.currentToken
	tm.mu.RUnlock()

	// If token has enough validity, return it
	if timeRemaining > minValidity {
		return token, nil
	}

	// Token is about to expire, trigger immediate refresh
	logger.Debug("Token expiring soon, triggering refresh",
		"remaining", timeRemaining,
		"min_validity", minValidity)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := tm.refresh(ctx); err != nil {
		// If refresh fails, return current token anyway (might still work)
		logger.Warn("Token refresh failed, using existing token", "error", err)
		return tm.GetToken(), err
	}

	return tm.GetToken(), nil
}

// TimeUntilExpiry returns how much time is left until token expires
func (tm *TokenManager) TimeUntilExpiry() time.Duration {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return time.Until(tm.tokenExpiry)
}

// ForceRefresh triggers an immediate token refresh
func (tm *TokenManager) ForceRefresh() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return tm.refresh(ctx)
}
