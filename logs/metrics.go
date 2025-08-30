package logs

import (
	"context"
	"time"

	"github.com/ELLIO-Technology/ellio_traefik_forward_auth/metrics"
)

type MetricsCollector struct {
	shipper *LogShipper
	buffer  *RingBuffer
	bucket  *LeakyBucket
	ctx     context.Context
	cancel  context.CancelFunc

	// Track last values for delta calculation
	lastEventsShipped  int64
	lastEventsDropped  int64
	lastShippingErrors int64
	lastBatchesSent    int64
}

func NewMetricsCollector(shipper *LogShipper, buffer *RingBuffer, bucket *LeakyBucket) *MetricsCollector {
	ctx, cancel := context.WithCancel(context.Background())
	return &MetricsCollector{
		shipper: shipper,
		buffer:  buffer,
		bucket:  bucket,
		ctx:     ctx,
		cancel:  cancel,
	}
}

func (mc *MetricsCollector) Start() {
	go mc.collectMetrics()
}

func (mc *MetricsCollector) Stop() {
	mc.cancel()
}

func (mc *MetricsCollector) collectMetrics() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-mc.ctx.Done():
			return
		case <-ticker.C:
			mc.updateMetrics()
		}
	}
}

func (mc *MetricsCollector) updateMetrics() {
	if mc.shipper != nil && mc.shipper.metrics != nil {
		shipperMetrics := mc.shipper.GetMetrics()

		// Update counter metrics
		metrics.LogEventsShippedTotal.Add(float64(shipperMetrics.EventsShipped.Load() - mc.lastEventsShipped))
		metrics.LogEventsDroppedTotal.Add(float64(shipperMetrics.EventsDropped.Load() - mc.lastEventsDropped))
		metrics.LogShippingErrorsTotal.Add(float64(shipperMetrics.ShippingErrors.Load() - mc.lastShippingErrors))
		metrics.LogBatchesSentTotal.Add(float64(shipperMetrics.BatchesSent.Load() - mc.lastBatchesSent))

		// Store last values for delta calculation
		mc.lastEventsShipped = shipperMetrics.EventsShipped.Load()
		mc.lastEventsDropped = shipperMetrics.EventsDropped.Load()
		mc.lastShippingErrors = shipperMetrics.ShippingErrors.Load()
		mc.lastBatchesSent = shipperMetrics.BatchesSent.Load()
	}

	// Update gauge metrics
	if mc.bucket != nil {
		metrics.LeakyBucketTokensAvailable.Set(float64(mc.bucket.AvailableTokens()))
	}

	if mc.buffer != nil {
		metrics.LogBufferSize.Set(float64(mc.buffer.Size()))
	}
}
