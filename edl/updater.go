package edl

import (
	"context"
	"errors"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"github.com/ELLIO-Technology/ellio_traefik_forward_auth/config"
	"github.com/ELLIO-Technology/ellio_traefik_forward_auth/metrics"
	"github.com/ELLIO-Technology/ellio_traefik_forward_auth/ipmatcher"
	"github.com/ELLIO-Technology/ellio_traefik_forward_auth/logger"
)

type Updater struct {
	fetcher     *Fetcher
	matcher     *ipmatcher.Matcher
	config      *config.Config
	lastUpdate  time.Time
	lastError   error
	updateCount int64
	mu          sync.RWMutex
}

func NewUpdater(cfg *config.Config, matcher *ipmatcher.Matcher) *Updater {
	return &Updater{
		fetcher: NewFetcher(cfg),
		matcher: matcher,
		config:  cfg,
	}
}

func (u *Updater) Start(ctx context.Context) error {
	// Skip EDL fetching if deployment is disabled
	if !u.config.DeploymentEnabled {
		return nil
	}

	if err := u.updateNow(ctx); err != nil {
		return errors.New("initial EDL fetch failed: " + err.Error())
	}

	go u.runUpdateLoop(ctx)
	return nil
}

func (u *Updater) runUpdateLoop(ctx context.Context) {
	ticker := time.NewTicker(u.config.UpdateFrequency)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := u.updateNow(ctx); err != nil {
				logger.Error("EDL update failed", "error", err)
			}
		}
	}
}

func (u *Updater) updateNow(ctx context.Context) error {
	start := time.Now()

	ipset, count, err := u.fetcher.FetchWithRetry(ctx)
	if err != nil {
		u.mu.Lock()
		u.lastError = err
		u.mu.Unlock()
		metrics.EDLUpdatesTotal.WithLabelValues("failure").Inc()
		return err
	}

	u.matcher.Update(ipset, count)

	u.mu.Lock()
	u.lastUpdate = time.Now()
	u.lastError = nil
	u.updateCount++
	u.mu.Unlock()

	// Update Prometheus metrics
	metrics.EDLEntries.Set(float64(count))
	metrics.EDLUpdatesTotal.WithLabelValues("success").Inc()
	metrics.EDLLastUpdateTimestamp.Set(float64(time.Now().Unix()))
	metrics.EDLUpdateDuration.Observe(time.Since(start).Seconds())

	if count == 0 {
		logger.Warn("EDL updated with empty list",
			"entries", 0,
			"duration", time.Since(start))
	} else {
		logger.Info("EDL updated successfully",
			"entries", count,
			"duration", time.Since(start))
	}

	// Force garbage collection and free memory back to OS
	runtime.GC()
	debug.FreeOSMemory()
	return nil
}

func (u *Updater) GetStatus() (time.Time, error, int64, int64) {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.lastUpdate, u.lastError, u.updateCount, u.matcher.Count()
}
