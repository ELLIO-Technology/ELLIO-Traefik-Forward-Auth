package edl

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/ELLIO-Technology/ellio_traefik_forward_auth/config"
	"go4.org/netipx"
	"github.com/ELLIO-Technology/ellio_traefik_forward_auth/logger"
	"github.com/getsentry/sentry-go"
)

type Fetcher struct {
	client *http.Client
	config *config.Config
}

func NewFetcher(cfg *config.Config) *Fetcher {
	return &Fetcher{
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				IdleConnTimeout:     30 * time.Second,
				DisableCompression:  true,
				MaxIdleConnsPerHost: 2,
			},
		},
		config: cfg,
	}
}

func (f *Fetcher) FetchWithRetry(ctx context.Context) (*netipx.IPSet, int64, error) {
	var lastErr error

	for attempt := 0; attempt < f.config.MaxRetryAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, 0, ctx.Err()
			case <-time.After(f.config.RetryDelay * time.Duration(attempt)):
			}
		}

		ipset, count, err := f.fetch(ctx)
		if err == nil {
			return ipset, count, nil
		}

		lastErr = err
		logger.Debug("EDL fetch attempt failed",
			"attempt", attempt+1,
			"max_attempts", f.config.MaxRetryAttempts,
			"error", err)
	}

	// Capture final failure to Sentry
	sentry.CaptureException(lastErr)
	return nil, 0, lastErr
}

func (f *Fetcher) fetch(ctx context.Context) (*netipx.IPSet, int64, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", f.config.EDLURL, nil)
	if err != nil {
		return nil, 0, err
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, 0, errors.New("unexpected status: " + string(body))
	}

	return f.parseEDL(resp.Body)
}

func (f *Fetcher) parseEDL(r io.Reader) (*netipx.IPSet, int64, error) {
	var b netipx.IPSetBuilder
	var count int64
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Try parsing as CIDR prefix first
		if prefix, err := netip.ParsePrefix(line); err == nil {
			b.AddPrefix(prefix)
			count++
		} else if addr, err := netip.ParseAddr(line); err == nil {
			// Single IP address
			b.Add(addr)
			count++
		}
		// Skip invalid entries silently
	}

	if err := scanner.Err(); err != nil {
		return nil, 0, err
	}

	if count == 0 {
		logger.Warn("EDL is empty - no IP addresses found")
	}

	ipset, err := b.IPSet()
	if err != nil {
		return nil, 0, err
	}

	return ipset, count, nil
}
