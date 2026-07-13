package api

import (
	"context"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"nav-saas-mvp/backend/internal/calcworker"
	"nav-saas-mvp/backend/internal/observability"
	"nav-saas-mvp/backend/internal/store"
)

func (s *Server) handleAdminMonitoring(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, buildMonitoringSnapshot(r.Context(), s.store, s.gsn != nil && s.gsn.Configured()))
}

func buildMonitoringSnapshot(ctx context.Context, fileStore *store.FileStore, gsnConfigured bool) map[string]any {
	authMetricsURL := envOr("APP_AUTH_METRICS_URL", "http://127.0.0.1:9091/metrics")
	workerMetricsURL := envOr("APP_WORKER_METRICS_URL", "http://127.0.0.1:9092/metrics")

	metrics, services := observability.CombinedSnapshot(authMetricsURL, workerMetricsURL)
	queue := buildQueueHealth(ctx, fileStore, gsnConfigured)

	avg := func(sum, count float64) float64 {
		if count <= 0 {
			return 0
		}
		return safeFloat(sum / count)
	}

	links := map[string]string{
		"grafana":    envOr("APP_GRAFANA_URL", "http://localhost:3000"),
		"prometheus": envOr("APP_PROMETHEUS_URL", "http://localhost:9093"),
		"k6":         envOr("APP_K6_GRAFANA_URL", "http://localhost:3000/d/nav-k6-load/k6-load-test"),
		"loki":       envOr("APP_LOKI_URL", "http://localhost:3000/explore?orgId=1&left=%7B%22datasource%22:%22loki%22,%22queries%22:%5B%7B%22expr%22:%22%7Bjob%3D%5C%22nav%5C%22%7D%22,%22refId%22:%22A%22%7D%5D,%22range%22:%7B%22from%22:%22now-1h%22,%22to%22:%22now%22%7D%7D"),
		"tempo":      envOr("APP_TEMPO_URL", "http://localhost:3000/explore?orgId=1&left=%7B%22datasource%22:%22tempo%22%7D"),
		"healthz":    "/api/healthz",
	}

	return map[string]any{
		"collectedAt": time.Now().UTC(),
		"services":    services,
		"links":       links,
		"http": map[string]any{
			"requestsTotal": safeFloat(metrics.HTTPRequestsTotal),
			"errorsTotal":   safeFloat(metrics.HTTPErrorsTotal),
		},
		"calc": map[string]any{
			"startsTotal":        safeFloat(metrics.EstimateCalcStarts),
			"processed":          safeFloat(sumMap(metrics.CalcProcessed)),
			"processedByResult":  safeFloatMap(metrics.CalcProcessed),
			"errorsByStage":      safeFloatMap(metrics.CalcErrors),
			"consumerProcessed":  pickConsumerCounter(metrics.ConsumerProcessed, queue, "processed"),
			"consumerRetried":    pickConsumerCounter(metrics.ConsumerRetried, queue, "retried"),
			"consumerDead":       pickConsumerCounter(metrics.ConsumerDead, queue, "dead"),
			"consumerFailed":     pickConsumerCounter(metrics.ConsumerFailed, queue, "failed"),
			"consumerDuplicates": pickConsumerCounter(metrics.ConsumerDuplicates, queue, "duplicates"),
			"avgDurationMs":      avg(metrics.CalcDurationSum, metrics.CalcDurationCount) * 1000,
			"avgGsnMs":           avg(metrics.GSNDurationSum, metrics.GSNDurationCount) * 1000,
			"avgPricingMs":       avg(metrics.PricingDurationSum, metrics.PricingDurationCount) * 1000,
			"samples": map[string]any{
				"total":   safeFloat(metrics.CalcDurationCount),
				"gsn":     safeFloat(metrics.GSNDurationCount),
				"pricing": safeFloat(metrics.PricingDurationCount),
			},
		},
		"queue": map[string]any{
			"status":          queue["status"],
			"pipelineReady":   queue["pipelineReady"],
			"releaseReady":    queue["releaseReady"],
			"outboxPending":   safeFloat(metrics.OutboxPending),
			"dlq":             nestedInt(queue, "dlq", "current"),
			"main":            pickQueueDepth(metrics.QueueDepths, queue, "estimate.calc.main"),
			"consumerAgeSec":  safeFloat(metrics.ConsumerHeartbeatAge),
			"publisherOnline": metrics.RabbitConnected["publisher"] > 0,
			"consumerOnline":  metrics.RabbitConnected["consumer"] > 0,
			"alerts":          queue["alerts"],
		},
	}
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func sumMap(values map[string]float64) float64 {
	var total float64
	for _, value := range values {
		total += value
	}
	return total
}

func safeFloat(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
}

func safeFloatMap(values map[string]float64) map[string]float64 {
	if len(values) == 0 {
		return map[string]float64{}
	}
	out := make(map[string]float64, len(values))
	for key, value := range values {
		out[key] = safeFloat(value)
	}
	return out
}

func pickQueueDepth(metrics map[string]float64, queue map[string]any, name string) float64 {
	if value, ok := metrics[name]; ok {
		return value
	}
	depths, _ := queue["queue"].(map[string]any)
	if depths == nil {
		return 0
	}
	rawDepths, _ := depths["depths"].(map[string]int)
	if rawDepths == nil {
		return 0
	}
	return float64(rawDepths[name])
}

func pickConsumerCounter(metricValue float64, queue map[string]any, field string) float64 {
	if metricValue > 0 {
		return metricValue
	}
	q, _ := queue["queue"].(map[string]any)
	if q == nil {
		return 0
	}
	switch consumer := q["consumer"].(type) {
	case calcworker.ConsumerStatus:
		switch field {
		case "processed":
			return float64(consumer.Processed)
		case "retried":
			return float64(consumer.Retried)
		case "dead":
			return float64(consumer.Dead)
		case "failed":
			return float64(consumer.Failed)
		case "duplicates":
			return float64(consumer.Duplicates)
		}
	case map[string]any:
		if value, ok := consumer[field]; ok {
			switch typed := value.(type) {
			case int64:
				return float64(typed)
			case int:
				return float64(typed)
			case float64:
				return typed
			}
		}
	}
	return 0
}

func nestedInt(queue map[string]any, keys ...string) float64 {
	current := any(queue)
	for _, key := range keys {
		asMap, ok := current.(map[string]any)
		if !ok {
			return 0
		}
		current = asMap[key]
	}
	switch value := current.(type) {
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case float64:
		return value
	default:
		return 0
	}
}
