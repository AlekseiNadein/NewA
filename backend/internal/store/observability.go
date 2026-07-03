package store

import (
	"context"
	"time"

	"nav-saas-mvp/backend/internal/observability"
)

// QueueMetricsBridge adapts FileStore for observability queue collectors.
type QueueMetricsBridge struct {
	Store *FileStore
}

// LoadQueueConsumerStats implements observability.ConsumerStatsSource.
func (b QueueMetricsBridge) LoadQueueConsumerStats(ctx context.Context) (observability.ConsumerStatsSnapshot, bool) {
	if b.Store == nil {
		return observability.ConsumerStatsSnapshot{}, false
	}
	stats, ok := b.Store.LoadQueueConsumerStats(ctx)
	if !ok {
		return observability.ConsumerStatsSnapshot{}, false
	}
	return observability.ConsumerStatsSnapshot{
		Processed:  stats.Processed,
		Retried:    stats.Retried,
		Dead:       stats.Dead,
		Failed:     stats.Failed,
		Duplicates: stats.Duplicates,
	}, true
}

// QueueConsumerHeartbeatAge implements observability.ConsumerStatsSource.
func (b QueueMetricsBridge) QueueConsumerHeartbeatAge(ctx context.Context) (time.Duration, bool) {
	if b.Store == nil {
		return 0, false
	}
	return b.Store.QueueConsumerHeartbeatAge(ctx)
}
