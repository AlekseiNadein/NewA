package calcworker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"nav-saas-mvp/backend/internal/gsn"
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
	RequestID      string    `json:"requestId"`
	Traceparent    string    `json:"traceparent"`
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
var consumerSessions atomic.Int32

type RabbitManager struct {
	store *store.FileStore
	gsn   *gsn.Service
	cfg   RabbitConfig
}

func NewRabbitManager(store *store.FileStore, gsnService *gsn.Service, cfg RabbitConfig) *RabbitManager {
	return &RabbitManager{
		store: store,
		gsn:   gsnService,
		cfg:   normalizeRabbitConfig(cfg),
	}
}

func (m *RabbitManager) Run(ctx context.Context) {
	if !m.gsn.Configured() {
		slog.Warn("GSN database is not configured; calc worker service is idle")
		<-ctx.Done()
		return
	}

	var cancel context.CancelFunc
	currentCount := 0
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	apply := func() {
		settings, err := m.store.GetAppSettings()
		if err != nil {
			slog.Warn("failed to read app settings", "error", err)
			return
		}
		nextCount := store.NormalizeCalcWorkerCount(settings.CalcWorkerCount)
		m.gsn.SetMaxOpenConns(nextCount + 4)
		if nextCount == currentCount {
			return
		}
		if cancel != nil {
			cancel()
		}
		var groupCtx context.Context
		groupCtx, cancel = context.WithCancel(ctx)
		startRabbitConsumerGroup(groupCtx, m.store, m.gsn, m.cfg, nextCount)
		slog.Info("rabbit calc worker group configured", "workers", nextCount, "prefetch", m.cfg.Prefetch)
		currentCount = nextCount
	}

	apply()
	for {
		select {
		case <-ctx.Done():
			if cancel != nil {
				cancel()
			}
			return
		case <-ticker.C:
			apply()
		}
	}
}

func startRabbitConsumerGroup(ctx context.Context, fileStore *store.FileStore, gsnService *gsn.Service, cfg RabbitConfig, count int) {
	worker := New(fileStore, gsnService)
	var wg sync.WaitGroup
	for i := 0; i < count; i += 1 {
		consumerTag := fmt.Sprintf("calc-worker-rabbit-%d", i+1)
		reportHeartbeat := i == 0
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker.runRabbitConsumer(ctx, cfg, consumerTag, reportHeartbeat)
		}()
	}
	go func() {
		<-ctx.Done()
		wg.Wait()
	}()
}

func beginConsumerSession() {
	if consumerSessions.Add(1) == 1 {
		consumerConnected.Store(true)
		observability.SetRabbitConnected("consumer", true)
	}
}

func endConsumerSession() {
	if consumerSessions.Add(-1) == 0 {
		consumerConnected.Store(false)
		observability.SetRabbitConnected("consumer", false)
	}
}

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
	w.runRabbitConsumer(ctx, cfg, "calc-worker-rabbit", true)
	return nil
}

func (w *Worker) runRabbitConsumer(ctx context.Context, cfg RabbitConfig, consumerTag string, reportHeartbeat bool) {
	for {
		if err := w.runRabbitSession(ctx, cfg, consumerTag, reportHeartbeat); err != nil && ctx.Err() == nil {
			consumerLastError.Store(err.Error())
			slog.Warn("rabbit consumer session failed", "consumer", consumerTag, "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
		if ctx.Err() != nil {
			return
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

func (w *Worker) runRabbitSession(ctx context.Context, cfg RabbitConfig, consumerTag string, reportHeartbeat bool) error {
	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return err
	}
	beginConsumerSession()
	consumerLastError.Store("")
	consumerLastMessageUnix.Store(time.Now().UTC().Unix())
	defer endConsumerSession()
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
	deliveries, err := ch.Consume(cfg.MainQueue, consumerTag, false, false, false, false, nil)
	if err != nil {
		return err
	}
	var heartbeat *time.Ticker
	var heartbeatC <-chan time.Time
	if reportHeartbeat {
		_ = w.store.TouchQueueConsumerHeartbeat(ctx)
		heartbeat = time.NewTicker(15 * time.Second)
		defer heartbeat.Stop()
		heartbeatC = heartbeat.C
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-heartbeatC:
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
		RequestID:   message.RequestID,
		Attempts:    message.Attempt,
		MaxAttempts: message.MaxAttempts,
	}
	logCtx := observability.WithRequestID(ctx, message.RequestID)
	logCtx = observability.ExtractAMQPHeaders(logCtx, delivery.Headers)
	logCtx = observability.ContextWithTraceParent(logCtx, message.Traceparent)
	if skip, reason := w.store.ShouldSkipCalcDelivery(logCtx, job); skip {
		consumerDuplicates.Add(1)
		observability.RecordCalcProcessed("duplicate")
		if reason == "receipt exists" {
			if err := w.store.ReconcileCalcLineFromReceipt(logCtx, job); err != nil {
				observability.LogCalcWarn(logCtx, "rabbit calc receipt reconcile failed", job.EstimateID, job.LineID, job.Code, job.ID, job.RequestID, "error", err)
			}
		} else if reason == "stale line revision" {
			_ = w.store.FailEstimateCalcJob(logCtx, job, errors.New("stale line revision"))
		}
		observability.LogCalcInfo(logCtx, "rabbit calc duplicate skipped", job.EstimateID, job.LineID, job.Code, job.ID, job.RequestID, "reason", reason)
		_ = delivery.Ack(false)
		return
	}
	if err := observability.RunCalcSpan(logCtx, job.EstimateID, job.LineID, job.ID, job.RequestID, message.Traceparent, func(spanCtx context.Context) error {
		return w.processJob(spanCtx, job)
	}); err != nil {
		consumerFailed.Add(1)
		observability.RecordCalcProcessed("failed")
		observability.LogCalcWarn(logCtx, "rabbit calc processing failed", job.EstimateID, job.LineID, job.Code, job.ID, job.RequestID, "error", err)
		_ = w.store.FailEstimateCalcJob(logCtx, job, err)
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
	observability.LogCalcInfo(logCtx, "rabbit calc completed", job.EstimateID, job.LineID, job.Code, job.ID, job.RequestID)
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
