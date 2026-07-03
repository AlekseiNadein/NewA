package observability

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

// MetricsSnapshot is a JSON-friendly view of selected Prometheus metrics.
type MetricsSnapshot struct {
	HTTPRequestsTotal    float64            `json:"httpRequestsTotal"`
	HTTPErrorsTotal      float64            `json:"httpErrorsTotal"`
	EstimateCalcStarts   float64            `json:"estimateCalcStarts"`
	OutboxPending        float64            `json:"outboxPending"`
	OutboxPublishedTotal float64            `json:"outboxPublishedTotal"`
	OutboxFailedTotal    float64            `json:"outboxFailedTotal"`
	RabbitConnected      map[string]float64 `json:"rabbitConnected"`
	QueueDepths          map[string]float64 `json:"queueDepths"`
	CalcProcessed        map[string]float64 `json:"calcProcessed"`
	CalcErrors           map[string]float64 `json:"calcErrors"`
	ConsumerProcessed    float64            `json:"consumerProcessed"`
	ConsumerRetried      float64            `json:"consumerRetried"`
	ConsumerDead         float64            `json:"consumerDead"`
	ConsumerFailed       float64            `json:"consumerFailed"`
	ConsumerDuplicates   float64            `json:"consumerDuplicates"`
	ConsumerHeartbeatAge float64            `json:"consumerHeartbeatAge"`
	CalcDurationCount    float64            `json:"calcDurationCount"`
	CalcDurationSum      float64            `json:"calcDurationSum"`
	GSNDurationCount     float64            `json:"gsnDurationCount"`
	GSNDurationSum       float64            `json:"gsnDurationSum"`
	PricingDurationCount float64            `json:"pricingDurationCount"`
	PricingDurationSum   float64            `json:"pricingDurationSum"`
}

// LocalSnapshot reads metrics from this process registry.
func LocalSnapshot() MetricsSnapshot {
	return snapshotFromFamilies(gatherFamilies(registry))
}

// ScrapeSnapshot fetches /metrics from another process.
func ScrapeSnapshot(metricsURL string, timeout time.Duration) (snap MetricsSnapshot, ok bool) {
	defer func() {
		if recover() != nil {
			snap = MetricsSnapshot{}
			ok = false
		}
	}()
	if strings.TrimSpace(metricsURL) == "" {
		return MetricsSnapshot{}, false
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(metricsURL)
	if err != nil {
		return MetricsSnapshot{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return MetricsSnapshot{}, false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return MetricsSnapshot{}, false
	}
	return snapshotFromFamilies(parseMetricFamilies(body)), true
}

func gatherFamilies(g prometheusGatherer) []*dto.MetricFamily {
	families, err := g.Gather()
	if err != nil {
		return nil
	}
	return families
}

type prometheusGatherer interface {
	Gather() ([]*dto.MetricFamily, error)
}

func parseMetricFamilies(body []byte) []*dto.MetricFamily {
	parser := expfmt.NewTextParser(model.UTF8Validation)
	families, err := parser.TextToMetricFamilies(bytes.NewReader(body))
	if err != nil {
		return nil
	}
	out := make([]*dto.MetricFamily, 0, len(families))
	for _, family := range families {
		out = append(out, family)
	}
	return out
}

func snapshotFromFamilies(families []*dto.MetricFamily) MetricsSnapshot {
	snap := MetricsSnapshot{
		RabbitConnected: make(map[string]float64),
		QueueDepths:     make(map[string]float64),
		CalcProcessed:   make(map[string]float64),
		CalcErrors:      make(map[string]float64),
	}
	for _, family := range families {
		if family == nil {
			continue
		}
		name := family.GetName()
		switch name {
		case "nav_http_requests_total":
			for _, metric := range family.Metric {
				status := labelValue(metric, "status")
				value := counterValue(metric)
				snap.HTTPRequestsTotal += value
				if strings.HasPrefix(status, "5") {
					snap.HTTPErrorsTotal += value
				}
			}
		case "nav_estimate_calc_start_total":
			snap.EstimateCalcStarts += sumCounterFamily(family)
		case "nav_outbox_pending":
			snap.OutboxPending += sumGaugeFamily(family)
		case "nav_outbox_published_total":
			snap.OutboxPublishedTotal += sumCounterFamily(family)
		case "nav_outbox_failed_total":
			snap.OutboxFailedTotal += sumCounterFamily(family)
		case "nav_rabbit_connected":
			for _, metric := range family.Metric {
				snap.RabbitConnected[labelValue(metric, "role")] = gaugeValue(metric)
			}
		case "nav_rabbit_queue_depth":
			for _, metric := range family.Metric {
				snap.QueueDepths[labelValue(metric, "queue")] = gaugeValue(metric)
			}
		case "nav_calc_processed_total":
			for _, metric := range family.Metric {
				snap.CalcProcessed[labelValue(metric, "result")] += counterValue(metric)
			}
		case "nav_calc_errors_total":
			for _, metric := range family.Metric {
				snap.CalcErrors[labelValue(metric, "stage")] += counterValue(metric)
			}
		case "nav_calc_consumer_processed":
			snap.ConsumerProcessed += sumGaugeFamily(family)
		case "nav_calc_consumer_retried":
			snap.ConsumerRetried += sumGaugeFamily(family)
		case "nav_calc_consumer_dead":
			snap.ConsumerDead += sumGaugeFamily(family)
		case "nav_calc_consumer_failed":
			snap.ConsumerFailed += sumGaugeFamily(family)
		case "nav_calc_consumer_duplicates":
			snap.ConsumerDuplicates += sumGaugeFamily(family)
		case "nav_calc_consumer_heartbeat_age_seconds":
			snap.ConsumerHeartbeatAge += sumGaugeFamily(family)
		case "nav_calc_duration_seconds":
			snap.CalcDurationCount, snap.CalcDurationSum = histogramTotals(family)
		case "nav_calc_gsn_duration_seconds":
			snap.GSNDurationCount, snap.GSNDurationSum = histogramTotals(family)
		case "nav_calc_pricing_duration_seconds":
			snap.PricingDurationCount, snap.PricingDurationSum = histogramTotals(family)
		}
	}
	return snap
}

func mergeSnapshots(base, extra MetricsSnapshot) MetricsSnapshot {
	for role, value := range extra.RabbitConnected {
		if _, exists := base.RabbitConnected[role]; !exists || value > 0 {
			base.RabbitConnected[role] = value
		}
	}
	for queue, value := range extra.QueueDepths {
		if _, exists := base.QueueDepths[queue]; !exists || value > 0 {
			base.QueueDepths[queue] = value
		}
	}
	for result, value := range extra.CalcProcessed {
		base.CalcProcessed[result] += value
	}
	for stage, value := range extra.CalcErrors {
		base.CalcErrors[stage] += value
	}
	if extra.ConsumerProcessed > 0 {
		base.ConsumerProcessed = extra.ConsumerProcessed
	}
	if extra.ConsumerRetried > 0 {
		base.ConsumerRetried = extra.ConsumerRetried
	}
	if extra.ConsumerDead > 0 {
		base.ConsumerDead = extra.ConsumerDead
	}
	if extra.ConsumerFailed > 0 {
		base.ConsumerFailed = extra.ConsumerFailed
	}
	if extra.ConsumerDuplicates > 0 {
		base.ConsumerDuplicates = extra.ConsumerDuplicates
	}
	if extra.ConsumerHeartbeatAge >= 0 {
		base.ConsumerHeartbeatAge = extra.ConsumerHeartbeatAge
	}
	if extra.CalcDurationCount > 0 {
		base.CalcDurationCount = extra.CalcDurationCount
		base.CalcDurationSum = extra.CalcDurationSum
	}
	if extra.GSNDurationCount > 0 {
		base.GSNDurationCount = extra.GSNDurationCount
		base.GSNDurationSum = extra.GSNDurationSum
	}
	if extra.PricingDurationCount > 0 {
		base.PricingDurationCount = extra.PricingDurationCount
		base.PricingDurationSum = extra.PricingDurationSum
	}
	return base
}

// CombinedSnapshot merges metrics from API, auth and calc worker endpoints.
func CombinedSnapshot(authURL, workerURL string) (MetricsSnapshot, map[string]bool) {
	services := map[string]bool{"api": true}
	snap := LocalSnapshot()
	if remote, ok := ScrapeSnapshot(authURL, 2*time.Second); ok {
		services["auth"] = true
		snap.HTTPRequestsTotal += remote.HTTPRequestsTotal
		snap.HTTPErrorsTotal += remote.HTTPErrorsTotal
	} else {
		services["auth"] = false
	}
	if remote, ok := ScrapeSnapshot(workerURL, 2*time.Second); ok {
		services["worker"] = true
		snap = mergeSnapshots(snap, remote)
	} else {
		services["worker"] = false
	}
	return snap, services
}

func sumCounterFamily(family *dto.MetricFamily) float64 {
	var total float64
	for _, metric := range family.Metric {
		total += counterValue(metric)
	}
	return total
}

func sumGaugeFamily(family *dto.MetricFamily) float64 {
	var total float64
	for _, metric := range family.Metric {
		total += gaugeValue(metric)
	}
	return total
}

func histogramTotals(family *dto.MetricFamily) (count float64, sum float64) {
	for _, metric := range family.Metric {
		if metric.Histogram == nil {
			continue
		}
		count += float64(metric.Histogram.GetSampleCount())
		sum += metric.Histogram.GetSampleSum()
	}
	return count, sum
}

func counterValue(metric *dto.Metric) float64 {
	if metric == nil || metric.Counter == nil {
		return 0
	}
	return metric.Counter.GetValue()
}

func gaugeValue(metric *dto.Metric) float64 {
	if metric == nil || metric.Gauge == nil {
		return 0
	}
	return metric.Gauge.GetValue()
}

func labelValue(metric *dto.Metric, name string) string {
	if metric == nil {
		return ""
	}
	for _, label := range metric.Label {
		if label.GetName() == name {
			return label.GetValue()
		}
	}
	return ""
}
