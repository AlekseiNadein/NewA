package observability

import (
	"context"
	"time"

	"nav-saas-mvp/backend/internal/queue"
)

// QueueGaugeSource supplies outbox backlog for Prometheus gauges.
type QueueGaugeSource interface {
	CountPendingOutboxEvents(ctx context.Context) (int64, error)
}

// ConsumerStatsSnapshot is the persisted calc worker counter set.
type ConsumerStatsSnapshot struct {
	Processed  int64
	Retried    int64
	Dead       int64
	Failed     int64
	Duplicates int64
}

// ConsumerStatsSource supplies persisted calc worker counters and heartbeat.
type ConsumerStatsSource interface {
	LoadQueueConsumerStats(ctx context.Context) (ConsumerStatsSnapshot, bool)
	QueueConsumerHeartbeatAge(ctx context.Context) (time.Duration, bool)
}

// QueueCollectorConfig configures periodic queue metric updates.
type QueueCollectorConfig struct {
	RabbitURL string
	Interval  time.Duration
	Source    QueueGaugeSource
	Consumer  ConsumerStatsSource
}

// RunQueueCollector updates queue-related gauges until ctx is cancelled.
func RunQueueCollector(ctx context.Context, cfg QueueCollectorConfig) {
	interval := cfg.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	update := func() {
		if cfg.Source != nil {
			if pending, err := cfg.Source.CountPendingOutboxEvents(ctx); err == nil {
				SetOutboxPending(pending)
			}
		}
		if cfg.Consumer != nil {
			if stats, ok := cfg.Consumer.LoadQueueConsumerStats(ctx); ok {
				SetConsumerStats(stats.Processed, stats.Retried, stats.Dead, stats.Failed, stats.Duplicates)
			}
			if age, ok := cfg.Consumer.QueueConsumerHeartbeatAge(ctx); ok {
				SetConsumerHeartbeatAge(age.Seconds())
				SetRabbitConnected("consumer", age <= 45*time.Second)
			} else {
				SetConsumerHeartbeatAge(-1)
				SetRabbitConnected("consumer", false)
			}
		}
		if cfg.RabbitURL == "" {
			return
		}
		depths, err := queue.InspectQueueDepths(cfg.RabbitURL)
		if err != nil {
			return
		}
		for name, depth := range depths {
			SetRabbitQueueDepth(name, depth)
		}
	}

	update()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			update()
		}
	}
}
