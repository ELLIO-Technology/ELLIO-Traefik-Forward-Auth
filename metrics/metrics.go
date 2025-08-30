package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	RequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "forwardauth_requests_total",
			Help: "Total number of auth requests",
		},
		[]string{"result"},
	)

	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "forwardauth_request_duration_seconds",
			Help:    "Request duration in seconds",
			Buckets: prometheus.ExponentialBuckets(0.0001, 2, 10),
		},
		[]string{"result"},
	)

	// EDL metrics
	EDLEntries = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "forwardauth_edl_entries",
			Help: "Current number of loaded EDL entries",
		},
	)

	EDLUpdatesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "forwardauth_edl_updates_total",
			Help: "Total number of EDL update attempts",
		},
		[]string{"status"},
	)

	EDLLastUpdateTimestamp = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "forwardauth_edl_last_update_timestamp",
			Help: "Unix timestamp of last successful EDL update",
		},
	)

	EDLUpdateDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "forwardauth_edl_update_duration_seconds",
			Help:    "EDL update operation duration in seconds",
			Buckets: prometheus.ExponentialBuckets(0.1, 2, 10),
		},
	)

	// Log shipping metrics
	LogEventsShippedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "forwardauth_log_events_shipped_total",
			Help: "Total number of log events successfully shipped",
		},
	)

	LogEventsDroppedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "forwardauth_log_events_dropped_total",
			Help: "Total number of log events dropped due to buffer overflow",
		},
	)

	LogShippingErrorsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "forwardauth_log_shipping_errors_total",
			Help: "Total number of log shipping errors",
		},
	)

	LogBatchesSentTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "forwardauth_log_batches_sent_total",
			Help: "Total number of log batches sent",
		},
	)

	LeakyBucketTokensAvailable = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "forwardauth_leaky_bucket_tokens_available",
			Help: "Current number of tokens available in the leaky bucket",
		},
	)

	LogBufferSize = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "forwardauth_log_buffer_size",
			Help: "Current number of events in the log buffer",
		},
	)
)
