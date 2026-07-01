package outbox

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"nav-saas-mvp/backend/internal/observability"
	"nav-saas-mvp/backend/internal/store"
)

type PublisherConfig struct {
	RabbitURL      string
	Exchange       string
	MainRoutingKey string
	DeadRoutingKey string
	RetryQueues    []RetryQueueConfig
	BatchSize      int
	Interval       time.Duration
}

type RetryQueueConfig struct {
	QueueName  string
	RoutingKey string
	TTL        time.Duration
}

var publisherConnected atomic.Bool
var publisherLastError atomic.Value
var publisherLastSuccessUnix atomic.Int64

type PublisherStatus struct {
	Connected   bool      `json:"connected"`
	LastError   string    `json:"lastError,omitempty"`
	LastSuccess time.Time `json:"lastSuccess,omitempty"`
	Published   int64     `json:"published"`
	Failed      int64     `json:"failed"`
}

var publisherPublished atomic.Int64
var publisherFailed atomic.Int64

func GetPublisherStatus() PublisherStatus {
	status := PublisherStatus{
		Connected: publisherConnected.Load(),
		Published: publisherPublished.Load(),
		Failed:    publisherFailed.Load(),
	}
	if value := publisherLastError.Load(); value != nil {
		if message, ok := value.(string); ok {
			status.LastError = message
		}
	}
	if ts := publisherLastSuccessUnix.Load(); ts > 0 {
		status.LastSuccess = time.Unix(ts, 0).UTC()
	}
	return status
}

func RunPublisher(ctx context.Context, fileStore *store.FileStore, cfg PublisherConfig) error {
	if fileStore == nil || strings.TrimSpace(cfg.RabbitURL) == "" {
		return nil
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.Interval <= 0 {
		cfg.Interval = time.Second
	}
	if cfg.Exchange == "" {
		cfg.Exchange = "estimate.calc"
	}
	if cfg.MainRoutingKey == "" {
		cfg.MainRoutingKey = "estimate.calc"
	}
	if cfg.DeadRoutingKey == "" {
		cfg.DeadRoutingKey = "estimate.calc.dead"
	}
	if len(cfg.RetryQueues) == 0 {
		cfg.RetryQueues = []RetryQueueConfig{
			{QueueName: "estimate.calc.retry.5s", RoutingKey: "estimate.calc.retry.5s", TTL: 5 * time.Second},
			{QueueName: "estimate.calc.retry.30s", RoutingKey: "estimate.calc.retry.30s", TTL: 30 * time.Second},
			{QueueName: "estimate.calc.retry.120s", RoutingKey: "estimate.calc.retry.120s", TTL: 120 * time.Second},
		}
	}
	for {
		if err := runPublisherSession(ctx, fileStore, cfg); err != nil && ctx.Err() == nil {
			publisherConnected.Store(false)
			publisherLastError.Store(err.Error())
			slog.Warn("outbox publisher session failed", "error", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(10 * time.Second):
			}
		}
		if ctx.Err() != nil {
			return nil
		}
	}
}

func runPublisherSession(ctx context.Context, fileStore *store.FileStore, cfg PublisherConfig) error {
	conn, err := amqp.Dial(cfg.RabbitURL)
	if err != nil {
		return err
	}
	publisherConnected.Store(true)
	publisherLastError.Store("")
	publisherLastSuccessUnix.Store(time.Now().UTC().Unix())
	observability.SetRabbitConnected("publisher", true)
	defer publisherConnected.Store(false)
	defer observability.SetRabbitConnected("publisher", false)
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	if err := declareTopology(ch, cfg); err != nil {
		return err
	}
	if err := ch.Confirm(false); err != nil {
		return err
	}
	ackCh := ch.NotifyPublish(make(chan amqp.Confirmation, cfg.BatchSize))
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()
	for {
		if err := publishBatch(ctx, fileStore, ch, ackCh, cfg); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func declareTopology(ch *amqp.Channel, cfg PublisherConfig) error {
	if err := ch.ExchangeDeclare(cfg.Exchange, "direct", true, false, false, false, nil); err != nil {
		return err
	}
	mainQueue := "estimate.calc.main"
	dlq := "estimate.calc.dlq"
	if _, err := ch.QueueDeclare(mainQueue, true, false, false, false, amqp.Table{
		"x-dead-letter-exchange":    cfg.Exchange,
		"x-dead-letter-routing-key": cfg.DeadRoutingKey,
	}); err != nil {
		return err
	}
	if err := ch.QueueBind(mainQueue, cfg.MainRoutingKey, cfg.Exchange, false, nil); err != nil {
		return err
	}
	if _, err := ch.QueueDeclare(dlq, true, false, false, false, nil); err != nil {
		return err
	}
	if err := ch.QueueBind(dlq, cfg.DeadRoutingKey, cfg.Exchange, false, nil); err != nil {
		return err
	}
	for _, retry := range cfg.RetryQueues {
		if retry.QueueName == "" || retry.RoutingKey == "" || retry.TTL <= 0 {
			continue
		}
		if _, err := ch.QueueDeclare(retry.QueueName, true, false, false, false, amqp.Table{
			"x-message-ttl":             int32(retry.TTL / time.Millisecond),
			"x-dead-letter-exchange":    cfg.Exchange,
			"x-dead-letter-routing-key": cfg.MainRoutingKey,
		}); err != nil {
			return err
		}
		if err := ch.QueueBind(retry.QueueName, retry.RoutingKey, cfg.Exchange, false, nil); err != nil {
			return err
		}
	}
	return nil
}

func publishBatch(ctx context.Context, fileStore *store.FileStore, ch *amqp.Channel, ackCh <-chan amqp.Confirmation, cfg PublisherConfig) error {
	events, err := fileStore.FetchPendingOutboxEvents(ctx, cfg.BatchSize)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}
	for _, evt := range events {
		err := ch.PublishWithContext(ctx, cfg.Exchange, evt.RoutingKey, false, false, amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         evt.Payload,
		})
		if err != nil {
			publisherFailed.Add(1)
			observability.RecordOutboxFailed()
			_ = fileStore.MarkOutboxEventFailed(ctx, evt.ID, err)
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case confirm := <-ackCh:
			if !confirm.Ack {
				publisherFailed.Add(1)
				observability.RecordOutboxFailed()
				_ = fileStore.MarkOutboxEventFailed(ctx, evt.ID, errors.New("rabbit publish was not acknowledged"))
				continue
			}
		}
		if err := fileStore.MarkOutboxEventPublished(ctx, evt.ID); err != nil {
			return err
		}
		publisherPublished.Add(1)
		observability.RecordOutboxPublished()
		publisherLastSuccessUnix.Store(time.Now().UTC().Unix())
	}
	return nil
}
