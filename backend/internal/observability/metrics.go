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

	calcDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "nav_calc_duration_seconds",
		Help:    "Estimate line calc processing duration in seconds.",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30},
	})
)

func init() {
	registry.MustRegister(
		httpRequestsTotal,
		httpRequestDuration,
		outboxPending,
		outboxPublishedTotal,
		outboxFailedTotal,
		rabbitConnected,
		rabbitQueueDepth,
		calcProcessedTotal,
		calcDuration,
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

func ObserveCalcDuration(seconds float64) {
	calcDuration.Observe(seconds)
}
