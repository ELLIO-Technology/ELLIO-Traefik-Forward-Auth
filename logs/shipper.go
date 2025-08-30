package logs

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ELLIO-Technology/ellio_traefik_forward_auth/utils"
	"github.com/ELLIO-Technology/ellio_traefik_forward_auth/logger"
	"github.com/getsentry/sentry-go"
)

const (
	defaultBatchSize        = 100
	defaultFlushInterval    = 10 * time.Second
	maxRetries              = 5
	initialBackoff          = 1 * time.Second
	maxBackoff              = 30 * time.Second
	circuitBreakerThreshold = 10
	circuitBreakerTimeout   = 60 * time.Second
)

type LogShipper struct {
	client        *http.Client
	tokenProvider TokenProvider
	bucket        *LeakyBucket

	eventChan chan *AccessEvent
	buffer    *RingBuffer

	batchSize     int
	flushInterval time.Duration

	failureCount atomic.Int32
	circuitOpen  atomic.Bool
	lastFailure  time.Time

	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc

	metrics *ShipperMetrics
}

type TokenProvider interface {
	GetToken() string
	GetLogsURL() string
}

type ShipperMetrics struct {
	EventsShipped  atomic.Int64
	EventsDropped  atomic.Int64
	ShippingErrors atomic.Int64
	BatchesSent    atomic.Int64
}

type LogShipperConfig struct {
	BatchSize      int
	FlushInterval  time.Duration
	BucketCapacity int64
	RefillRate     int64
	BufferSize     int
}

func NewLogShipper(tokenProvider TokenProvider, config *LogShipperConfig) *LogShipper {
	if config.BatchSize <= 0 {
		config.BatchSize = defaultBatchSize
	}
	if config.FlushInterval <= 0 {
		config.FlushInterval = defaultFlushInterval
	}
	if config.BucketCapacity <= 0 {
		config.BucketCapacity = 1000
	}
	if config.RefillRate <= 0 {
		config.RefillRate = 100
	}
	if config.BufferSize <= 0 {
		config.BufferSize = 10000
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &LogShipper{
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				IdleConnTimeout:     30 * time.Second,
				MaxIdleConnsPerHost: 2,
			},
		},
		tokenProvider: tokenProvider,
		bucket:        NewLeakyBucket(config.BucketCapacity, config.RefillRate),
		eventChan:     make(chan *AccessEvent, 1000),
		buffer:        NewRingBuffer(config.BufferSize),
		batchSize:     config.BatchSize,
		flushInterval: config.FlushInterval,
		ctx:           ctx,
		cancel:        cancel,
		metrics:       &ShipperMetrics{},
	}
}

func (s *LogShipper) Start() {
	s.wg.Add(1)
	go s.processEvents()
}

func (s *LogShipper) Stop() error {
	s.cancel()
	close(s.eventChan)

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.flushBuffer()
		return nil
	case <-time.After(10 * time.Second):
		return errors.New("timeout waiting for log shipper to stop")
	}
}

func (s *LogShipper) SendEvent(event *AccessEvent) {
	select {
	case s.eventChan <- event:
	default:
		if !s.buffer.Add(event) {
			s.metrics.EventsDropped.Add(1)
			logger.Warn("Event dropped: buffer full")
		}
	}
}

func (s *LogShipper) processEvents() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.flushInterval)
	defer ticker.Stop()

	batch := make([]*AccessEvent, 0, s.batchSize)

	for {
		select {
		case <-s.ctx.Done():
			if len(batch) > 0 {
				s.shipBatch(batch)
			}
			return

		case event, ok := <-s.eventChan:
			if !ok {
				if len(batch) > 0 {
					s.shipBatch(batch)
				}
				return
			}

			batch = append(batch, event)
			if len(batch) >= s.batchSize {
				s.shipBatch(batch)
				batch = make([]*AccessEvent, 0, s.batchSize)
			}

		case <-ticker.C:
			if len(batch) > 0 {
				s.shipBatch(batch)
				batch = make([]*AccessEvent, 0, s.batchSize)
			}

			s.processBufferedEvents()
		}
	}
}

func (s *LogShipper) processBufferedEvents() {
	events := s.buffer.Drain(s.batchSize)
	if len(events) > 0 {
		s.shipBatch(events)
	}
}

func (s *LogShipper) shipBatch(events []*AccessEvent) {
	if s.isCircuitOpen() {
		for _, event := range events {
			if !s.buffer.Add(event) {
				s.metrics.EventsDropped.Add(int64(1))
			}
		}
		return
	}

	waitTime := s.bucket.WaitTime(1)
	if waitTime > 0 {
		time.Sleep(waitTime)
	}

	if !s.bucket.Allow(1) {
		for _, event := range events {
			if !s.buffer.Add(event) {
				s.metrics.EventsDropped.Add(int64(1))
			}
		}
		return
	}

	// Convert to JSONL format
	payload, err := s.eventsToJSONL(events)
	if err != nil {
		logger.Error("Failed to convert events to JSONL", "error", err)
		s.metrics.EventsDropped.Add(int64(len(events)))
		return
	}

	err = s.sendWithRetry(payload)
	if err != nil {
		s.recordFailure()
		s.metrics.ShippingErrors.Add(1)
		logger.Error("Failed to ship batch",
			"events", len(events),
			"error", err)
		sentry.CaptureException(err)

		for _, event := range events {
			if !s.buffer.Add(event) {
				s.metrics.EventsDropped.Add(int64(1))
			}
		}
	} else {
		s.recordSuccess()
		s.metrics.EventsShipped.Add(int64(len(events)))
		s.metrics.BatchesSent.Add(1)
	}
}

func (s *LogShipper) sendWithRetry(payload []byte) error {
	var lastErr error
	backoff := initialBackoff

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(backoff)
			backoff = utils.MinDuration(backoff*2, maxBackoff)
		}

		err := s.send(payload)
		if err == nil {
			return nil
		}

		lastErr = err

		if !isRetryableError(err) {
			return err
		}
	}

	return lastErr
}

func (s *LogShipper) send(payload []byte) error {
	logsURL := s.tokenProvider.GetLogsURL()
	if logsURL == "" {
		return errors.New("logs URL not available")
	}

	token := s.tokenProvider.GetToken()
	if token == "" {
		return errors.New("access token not available")
	}

	// payload is already prepared as JSONL

	var body io.Reader
	headers := map[string]string{
		"Content-Type":  "application/x-ndjson", // JSONL content type
		"Authorization": "Bearer " + token,
	}

	if len(payload) > 1024 {
		compressed, err := compressPayload(payload)
		if err == nil {
			body = bytes.NewReader(compressed)
			headers["Content-Encoding"] = "gzip"
		} else {
			body = bytes.NewReader(payload)
		}
	} else {
		body = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(s.ctx, "POST", logsURL, body)
	if err != nil {
		return errors.New("failed to create request: " + err.Error())
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return errors.New("request failed: " + err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return errors.New("server error: " + string(bodyBytes))
}

func (s *LogShipper) isCircuitOpen() bool {
	if !s.circuitOpen.Load() {
		return false
	}

	if time.Since(s.lastFailure) > circuitBreakerTimeout {
		s.circuitOpen.Store(false)
		s.failureCount.Store(0)
		return false
	}

	return true
}

func (s *LogShipper) recordFailure() {
	count := s.failureCount.Add(1)
	s.lastFailure = time.Now()

	if count >= circuitBreakerThreshold {
		s.circuitOpen.Store(true)
		logger.Debug("Circuit breaker opened", "failures", count)
	}
}

func (s *LogShipper) recordSuccess() {
	s.failureCount.Store(0)
	s.circuitOpen.Store(false)
}

func (s *LogShipper) flushBuffer() {
	events := s.buffer.DrainAll()

	for len(events) > 0 {
		batchSize := utils.MinInt(len(events), s.batchSize)
		batch := events[:batchSize]
		events = events[batchSize:]

		s.shipBatch(batch)
	}
}

func (s *LogShipper) GetMetrics() *ShipperMetrics {
	return s.metrics
}

func compressPayload(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)

	_, err := gz.Write(data)
	if err != nil {
		return nil, err
	}

	if err := gz.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func isRetryableError(_ error) bool {
	return true
}

// Convert events to JSONL format (newline-delimited JSON)
func (s *LogShipper) eventsToJSONL(events []*AccessEvent) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)

	for _, event := range events {
		if err := encoder.Encode(event); err != nil {
			return nil, errors.New("failed to encode event: " + err.Error())
		}
	}

	return buf.Bytes(), nil
}
