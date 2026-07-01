package observability

import (
	"context"
	"time"

	"nav-saas-mvp/backend/internal/queue"
)

// QueueGaugeSource supplies queue-related values for Prometheus gauges.
type QueueGaugeSource interface {
	CountPendingOutboxEvents(ctx context.Context) (int64, error)
}

// QueueCollectorConfig configures periodic queue metric updates.
type QueueCollectorConfig struct {
	RabbitURL string
	Interval  time.Duration
	Source    QueueGaugeSource
}

// RunQueueCollector updates outbox and RabbitMQ depth gauges until ctx is cancelled.
func RunQueueCollector(ctx context.Context, cfg QueueCollectorConfig) {
	if cfg.Source == nil {
		return
	}
	interval := cfg.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	update := func() {
		if pending, err := cfg.Source.CountPendingOutboxEvents(ctx); err == nil {
			SetOutboxPending(pending)
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
