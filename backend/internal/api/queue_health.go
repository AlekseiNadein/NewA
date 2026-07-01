package api

import (
	"context"
	"os"
	"time"

	"nav-saas-mvp/backend/internal/calcworker"
	"nav-saas-mvp/backend/internal/outbox"
	"nav-saas-mvp/backend/internal/queue"
	"nav-saas-mvp/backend/internal/store"
)

const dlqGrowthAlertThreshold = 50

func buildQueueHealth(ctx context.Context, fileStore *store.FileStore, gsnConfigured bool) map[string]any {
	mode := fileStore.QueueMode()
	publisher := outbox.GetPublisherStatus()
	consumer := calcworker.GetConsumerStatus()
	if mode == "rabbit" {
		if age, ok := fileStore.QueueConsumerHeartbeatAge(ctx); ok && age <= 45*time.Second {
			consumer.Connected = true
			consumer.LastMessage = time.Now().UTC().Add(-age)
		} else {
			consumer.Connected = false
		}
	}
	outboxPending, _ := fileStore.CountPendingOutboxEvents(ctx)
	queueDepths, queueErr := queue.InspectQueueDepths(os.Getenv("APP_RABBITMQ_URL"))
	if queueErr != nil {
		queueDepths = map[string]int{}
	}
	dlqDepth := queueDepths["estimate.calc.dlq"]
	mainDepth := queueDepths["estimate.calc.main"]
	alerts := map[string]bool{
		"publisherDisconnected": (mode == "rabbit" || mode == "dual") && !publisher.Connected,
		"consumerDisconnected":  mode == "rabbit" && !consumer.Connected,
		"outboxBacklogHigh":     outboxPending > 100,
		"dlqNotEmpty":           dlqDepth > 0,
	}
	status := "ok"
	for key, active := range alerts {
		if !active || key == "dlqNotEmpty" {
			continue
		}
		status = "degraded"
		break
	}
	pipelineReady := !alerts["publisherDisconnected"] && !alerts["consumerDisconnected"] && !alerts["outboxBacklogHigh"]
	history, _ := fileStore.RecordQueueStatsSample(ctx, dlqDepth, mainDepth, outboxPending)
	dlqGrowth10m, hasGrowthBaseline := fileStore.QueueDLQDelta(ctx, 10*time.Minute, dlqDepth)
	dlqGrowthAlert := hasGrowthBaseline && dlqGrowth10m >= dlqGrowthAlertThreshold
	if dlqGrowthAlert {
		alerts["dlqGrowthHigh"] = true
	} else {
		alerts["dlqGrowthHigh"] = false
	}

	return map[string]any{
		"status": status,
		"queue": map[string]any{
			"mode":          mode,
			"publisher":     publisher,
			"consumer":      consumer,
			"outboxPending": outboxPending,
			"depths":        queueDepths,
		},
		"alerts":        alerts,
		"pipelineReady": pipelineReady,
		"releaseReady":  pipelineReady && mode == "rabbit" && !alerts["dlqNotEmpty"],
		"gsnConfigured": gsnConfigured,
		"dlq": map[string]any{
			"current":        dlqDepth,
			"growth10m":      dlqGrowth10m,
			"hasGrowthBaseline": hasGrowthBaseline,
			"growthAlert":    dlqGrowthAlert,
			"growthThreshold": dlqGrowthAlertThreshold,
		},
		"history": history,
	}
}
