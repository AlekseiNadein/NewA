package calcworker

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"nav-saas-mvp/backend/internal/observability"
	"nav-saas-mvp/backend/internal/store"
)

type RabbitConfig struct {
	URL              string
	Exchange         string
	MainQueue        string
	MainRoutingKey   string
	DeadQueue        string
	DeadRoutingKey   string
	RetryQueues      []string
	RetryRoutingKeys []string
	Prefetch         int
}

type rabbitJobMessage struct {
	MessageVersion int       `json:"messageVersion"`
	JobID          string    `json:"jobId"`
	CompanyID      string    `json:"companyId"`
	EstimateID     string    `json:"estimateId"`
	LineID         string    `json:"lineId"`
	Revision       int64     `json:"revision"`
	Code           string    `json:"code"`
	FgisSetID      string    `json:"fgisSetId"`
	District       string    `json:"district"`
	Quantity       float64   `json:"quantity"`
	RawText        string    `json:"rawText"`
	Attempt        int       `json:"attempt"`
	MaxAttempts    int       `json:"maxAttempts"`
	CreatedAt      time.Time `json:"createdAt"`
}

var consumerConnected atomic.Bool
var consumerLastError atomic.Value
var consumerLastMessageUnix atomic.Int64

type ConsumerStatus struct {
	Connected   bool      `json:"connected"`
	LastError   string    `json:"lastError,omitempty"`
	LastMessage time.Time `json:"lastMessage,omitempty"`
	Processed   int64     `json:"processed"`
	Retried     int64     `json:"retried"`
	Dead        int64     `json:"dead"`
	Failed      int64     `json:"failed"`
	Duplicates  int64     `json:"duplicates"`
}

var consumerProcessed atomic.Int64
var consumerRetried atomic.Int64
var consumerDead atomic.Int64
var consumerFailed atomic.Int64
var consumerDuplicates atomic.Int64

func GetConsumerStatus() ConsumerStatus {
	status := ConsumerStatus{
		Connected: consumerConnected.Load(),
		Processed: consumerProcessed.Load(),
		Retried:   consumerRetried.Load(),
		Dead:      consumerDead.Load(),
		Failed:    consumerFailed.Load(),
		Duplicates: consumerDuplicates.Load(),
	}
	if value := consumerLastError.Load(); value != nil {
		if message, ok := value.(string); ok {
			status.LastError = message
		}
	}
	if ts := consumerLastMessageUnix.Load(); ts > 0 {
		status.LastMessage = time.Unix(ts, 0).UTC()
	}
	return status
}

func (w *Worker) RunRabbit(ctx context.Context, cfg RabbitConfig) error {
	cfg = normalizeRabbitConfig(cfg)
	for {
		if err := w.runRabbitSession(ctx, cfg); err != nil && ctx.Err() == nil {
			consumerConnected.Store(false)
			consumerLastError.Store(err.Error())
			slog.Warn("rabbit consumer session failed", "error", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(5 * time.Second):
			}
		}
		if ctx.Err() != nil {
			return nil
		}
	}
}

func normalizeRabbitConfig(cfg RabbitConfig) RabbitConfig {
	if cfg.Exchange == "" {
		cfg.Exchange = "estimate.calc"
	}
	if cfg.MainQueue == "" {
		cfg.MainQueue = "estimate.calc.main"
	}
	if cfg.MainRoutingKey == "" {
		cfg.MainRoutingKey = "estimate.calc"
	}
	if cfg.DeadQueue == "" {
		cfg.DeadQueue = "estimate.calc.dlq"
	}
	if cfg.DeadRoutingKey == "" {
		cfg.DeadRoutingKey = "estimate.calc.dead"
	}
	if len(cfg.RetryQueues) == 0 {
		cfg.RetryQueues = []string{
			"estimate.calc.retry.5s",
			"estimate.calc.retry.30s",
			"estimate.calc.retry.120s",
		}
	}
	if len(cfg.RetryRoutingKeys) == 0 {
		cfg.RetryRoutingKeys = []string{
			"estimate.calc.retry.5s",
			"estimate.calc.retry.30s",
			"estimate.calc.retry.120s",
		}
	}
	if cfg.Prefetch <= 0 {
		cfg.Prefetch = 8
	}
	if cfg.Prefetch > 32 {
		cfg.Prefetch = 32
	}
	return cfg
}

func NormalizeRabbitPrefetch(value int) int {
	if value < 1 {
		return 4
	}
	if value > 32 {
		return 32
	}
	return value
}

func (w *Worker) runRabbitSession(ctx context.Context, cfg RabbitConfig) error {
	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return err
	}
	consumerConnected.Store(true)
	consumerLastError.Store("")
	consumerLastMessageUnix.Store(time.Now().UTC().Unix())
	observability.SetRabbitConnected("consumer", true)
	defer consumerConnected.Store(false)
	defer observability.SetRabbitConnected("consumer", false)
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	if err := declareRabbitTopology(ch, cfg); err != nil {
		return err
	}
	if err := ch.Qos(cfg.Prefetch, 0, false); err != nil {
		return err
	}
	deliveries, err := ch.Consume(cfg.MainQueue, "calc-worker-rabbit", false, false, false, false, nil)
	if err != nil {
		return err
	}
	_ = w.store.TouchQueueConsumerHeartbeat(ctx)
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-heartbeat.C:
			_ = w.store.TouchQueueConsumerHeartbeat(ctx)
			_ = w.store.SaveQueueConsumerStats(ctx, store.QueueConsumerStats{
				Processed:  consumerProcessed.Load(),
				Retried:    consumerRetried.Load(),
				Dead:       consumerDead.Load(),
				Failed:     consumerFailed.Load(),
				Duplicates: consumerDuplicates.Load(),
			})
		case msg, ok := <-deliveries:
			if !ok {
				return amqp.ErrClosed
			}
			consumerLastMessageUnix.Store(time.Now().UTC().Unix())
			w.handleRabbitDelivery(ctx, ch, cfg, msg)
		}
	}
}

func declareRabbitTopology(ch *amqp.Channel, cfg RabbitConfig) error {
	if err := ch.ExchangeDeclare(cfg.Exchange, "direct", true, false, false, false, nil); err != nil {
		return err
	}
	if _, err := ch.QueueDeclare(cfg.MainQueue, true, false, false, false, amqp.Table{
		"x-dead-letter-exchange":    cfg.Exchange,
		"x-dead-letter-routing-key": cfg.DeadRoutingKey,
	}); err != nil {
		return err
	}
	if err := ch.QueueBind(cfg.MainQueue, cfg.MainRoutingKey, cfg.Exchange, false, nil); err != nil {
		return err
	}
	if _, err := ch.QueueDeclare(cfg.DeadQueue, true, false, false, false, nil); err != nil {
		return err
	}
	if err := ch.QueueBind(cfg.DeadQueue, cfg.DeadRoutingKey, cfg.Exchange, false, nil); err != nil {
		return err
	}
	ttls := []int32{5000, 30000, 120000}
	for i, queue := range cfg.RetryQueues {
		if i >= len(cfg.RetryRoutingKeys) || strings.TrimSpace(queue) == "" {
			continue
		}
		ttl := ttls[len(ttls)-1]
		if i < len(ttls) {
			ttl = ttls[i]
		}
		if _, err := ch.QueueDeclare(queue, true, false, false, false, amqp.Table{
			"x-message-ttl":             ttl,
			"x-dead-letter-exchange":    cfg.Exchange,
			"x-dead-letter-routing-key": cfg.MainRoutingKey,
		}); err != nil {
			return err
		}
		if err := ch.QueueBind(queue, cfg.RetryRoutingKeys[i], cfg.Exchange, false, nil); err != nil {
			return err
		}
	}
	return nil
}

func (w *Worker) handleRabbitDelivery(ctx context.Context, ch *amqp.Channel, cfg RabbitConfig, delivery amqp.Delivery) {
	var message rabbitJobMessage
	if err := json.Unmarshal(delivery.Body, &message); err != nil {
		slog.Warn("invalid rabbit calc message", "error", err)
		_ = delivery.Reject(false)
		return
	}
	if message.MaxAttempts <= 0 {
		message.MaxAttempts = 5
	}
	if message.Attempt <= 0 {
		message.Attempt = 1
	}
	job := store.EstimateCalcJob{
		ID:          message.JobID,
		CompanyID:   message.CompanyID,
		EstimateID:  message.EstimateID,
		LineID:      message.LineID,
		Revision:    message.Revision,
		Code:        message.Code,
		FgisSetID:   message.FgisSetID,
		District:    message.District,
		Quantity:    message.Quantity,
		RawText:     message.RawText,
		Attempts:    message.Attempt,
		MaxAttempts: message.MaxAttempts,
	}
	if skip, reason := w.store.ShouldSkipCalcDelivery(ctx, job); skip {
		consumerDuplicates.Add(1)
		observability.RecordCalcProcessed("duplicate")
		if reason == "receipt exists" {
			if err := w.store.ReconcileCalcLineFromReceipt(ctx, job); err != nil {
				slog.Warn("rabbit calc receipt reconcile failed", "job", job.ID, "estimate", job.EstimateID, "line", job.LineID, "error", err)
			}
		}
		slog.Info("rabbit calc duplicate skipped", "job", job.ID, "estimate", job.EstimateID, "line", job.LineID, "reason", reason)
		_ = delivery.Ack(false)
		return
	}
	if err := w.processJob(ctx, job); err != nil {
		consumerFailed.Add(1)
		observability.RecordCalcProcessed("failed")
		slog.Warn("rabbit calc processing failed", "job", job.ID, "error", err)
		_ = w.store.FailEstimateCalcJob(ctx, job, err)
		message.Attempt++
		routingKey := cfg.DeadRoutingKey
		permanent := isPermanentCalcError(err)
		if !permanent && message.Attempt <= message.MaxAttempts {
			routingKey = pickRetryRoutingKey(message.Attempt, cfg)
			consumerRetried.Add(1)
			observability.RecordCalcProcessed("retry")
		} else {
			consumerDead.Add(1)
			observability.RecordCalcProcessed("dead")
		}
		body, marshalErr := json.Marshal(message)
		if marshalErr != nil {
			_ = delivery.Reject(false)
			return
		}
		publishErr := ch.PublishWithContext(ctx, cfg.Exchange, routingKey, false, false, amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
		})
		if publishErr != nil {
			_ = delivery.Nack(false, true)
			return
		}
		_ = delivery.Ack(false)
		return
	}
	consumerProcessed.Add(1)
	observability.RecordCalcProcessed("ok")
	_ = delivery.Ack(false)
}

func isPermanentCalcError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "record not found") || strings.Contains(message, "stale line revision")
}

func pickRetryRoutingKey(attempt int, cfg RabbitConfig) string {
	if len(cfg.RetryRoutingKeys) == 0 {
		return cfg.DeadRoutingKey
	}
	switch {
	case attempt <= 2:
		return cfg.RetryRoutingKeys[0]
	case attempt <= 4 && len(cfg.RetryRoutingKeys) >= 2:
		return cfg.RetryRoutingKeys[1]
	case len(cfg.RetryRoutingKeys) >= 3:
		return cfg.RetryRoutingKeys[2]
	default:
		return cfg.RetryRoutingKeys[len(cfg.RetryRoutingKeys)-1]
	}
}
