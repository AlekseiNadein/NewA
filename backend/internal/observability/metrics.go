package observability

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	registry = prometheus.NewRegistry()

	httpRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "nav_http_requests_total",
		Help: "Total HTTP requests handled by the service.",
	}, []string{"method", "route", "status"})

	httpRequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "nav_http_request_duration_seconds",
		Help:    "HTTP request latency in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route"})

	estimateCalcStartTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "nav_estimate_calc_start_total",
		Help: "Total estimate calc start requests accepted by the API.",
	})

	outboxPending = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "nav_outbox_pending",
		Help: "Number of pending outbox events waiting to be published.",
	})

	outboxPublishedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "nav_outbox_published_total",
		Help: "Total number of outbox events published to RabbitMQ.",
	})

	outboxFailedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "nav_outbox_failed_total",
		Help: "Total number of outbox publish failures.",
	})

	rabbitConnected = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nav_rabbit_connected",
		Help: "RabbitMQ connection state (1 connected, 0 disconnected).",
	}, []string{"role"})

	rabbitQueueDepth = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nav_rabbit_queue_depth",
		Help: "Current RabbitMQ queue depth.",
	}, []string{"queue"})

	calcProcessedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "nav_calc_processed_total",
		Help: "Total calc jobs processed by result type.",
	}, []string{"result"})

	calcErrorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "nav_calc_errors_total",
		Help: "Total calc job failures by processing stage.",
	}, []string{"stage"})

	calcDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "nav_calc_duration_seconds",
		Help:    "Estimate line calc processing duration in seconds.",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 40},
	})

	calcGSNDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "nav_calc_gsn_duration_seconds",
		Help:    "GSN record lookup duration in seconds.",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 40},
	})

	calcPricingDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "nav_calc_pricing_duration_seconds",
		Help:    "Estimate line pricing duration in seconds.",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
	})

	consumerProcessedGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "nav_calc_consumer_processed",
		Help: "Persisted calc worker processed counter.",
	})
	consumerRetriedGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "nav_calc_consumer_retried",
		Help: "Persisted calc worker retried counter.",
	})
	consumerDeadGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "nav_calc_consumer_dead",
		Help: "Persisted calc worker dead-letter counter.",
	})
	consumerFailedGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "nav_calc_consumer_failed",
		Help: "Persisted calc worker failed counter.",
	})
	consumerDuplicatesGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "nav_calc_consumer_duplicates",
		Help: "Persisted calc worker duplicate counter.",
	})
	consumerHeartbeatAge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "nav_calc_consumer_heartbeat_age_seconds",
		Help: "Age of the last calc worker heartbeat in seconds (-1 if missing).",
	})

	gsnCacheHitsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "nav_gsn_cache_hits_total",
		Help: "Total GSN record detail cache hits.",
	})

	gsnCacheMissesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "nav_gsn_cache_misses_total",
		Help: "Total GSN record detail cache misses.",
	})

	gsnCacheErrorsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "nav_gsn_cache_errors_total",
		Help: "Total GSN record detail cache errors.",
	})
)

func init() {
	registry.MustRegister(
		httpRequestsTotal,
		httpRequestDuration,
		estimateCalcStartTotal,
		outboxPending,
		outboxPublishedTotal,
		outboxFailedTotal,
		rabbitConnected,
		rabbitQueueDepth,
		calcProcessedTotal,
		calcErrorsTotal,
		calcDuration,
		calcGSNDuration,
		calcPricingDuration,
		consumerProcessedGauge,
		consumerRetriedGauge,
		consumerDeadGauge,
		consumerFailedGauge,
		consumerDuplicatesGauge,
		consumerHeartbeatAge,
		gsnCacheHitsTotal,
		gsnCacheMissesTotal,
		gsnCacheErrorsTotal,
	)
}

// MetricsHandler exposes Prometheus metrics for scraping.
func MetricsHandler() http.Handler {
	return promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
}

func RecordHTTPRequest(method, route, status string, durationSeconds float64) {
	if route == "" {
		route = "unknown"
	}
	httpRequestsTotal.WithLabelValues(method, route, status).Inc()
	httpRequestDuration.WithLabelValues(method, route).Observe(durationSeconds)
}

func RecordEstimateCalcStart() {
	estimateCalcStartTotal.Inc()
}

func SetOutboxPending(value int64) {
	outboxPending.Set(float64(value))
}

func RecordOutboxPublished() {
	outboxPublishedTotal.Inc()
}

func RecordOutboxFailed() {
	outboxFailedTotal.Inc()
}

func SetRabbitConnected(role string, connected bool) {
	value := 0.0
	if connected {
		value = 1
	}
	rabbitConnected.WithLabelValues(role).Set(value)
}

func SetRabbitQueueDepth(queue string, depth int) {
	rabbitQueueDepth.WithLabelValues(queue).Set(float64(depth))
}

func RecordCalcProcessed(result string) {
	calcProcessedTotal.WithLabelValues(result).Inc()
}

func RecordCalcError(stage string) {
	if stage == "" {
		stage = "unknown"
	}
	calcErrorsTotal.WithLabelValues(stage).Inc()
}

func ObserveCalcDuration(seconds float64) {
	calcDuration.Observe(seconds)
}

func ObserveCalcGSNDuration(seconds float64) {
	calcGSNDuration.Observe(seconds)
}

func ObserveCalcPricingDuration(seconds float64) {
	calcPricingDuration.Observe(seconds)
}

func SetConsumerStats(processed, retried, dead, failed, duplicates int64) {
	consumerProcessedGauge.Set(float64(processed))
	consumerRetriedGauge.Set(float64(retried))
	consumerDeadGauge.Set(float64(dead))
	consumerFailedGauge.Set(float64(failed))
	consumerDuplicatesGauge.Set(float64(duplicates))
}

func SetConsumerHeartbeatAge(seconds float64) {
	consumerHeartbeatAge.Set(seconds)
}

func RecordGSNCacheHit() {
	gsnCacheHitsTotal.Inc()
}

func RecordGSNCacheMiss() {
	gsnCacheMissesTotal.Inc()
}

func RecordGSNCacheError() {
	gsnCacheErrorsTotal.Inc()
}
