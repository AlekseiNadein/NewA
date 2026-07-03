package calcworker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"nav-saas-mvp/backend/internal/estimatecalc"
	"nav-saas-mvp/backend/internal/gsn"
	"nav-saas-mvp/backend/internal/observability"
	"nav-saas-mvp/backend/internal/store"
)

const defaultLease = 45 * time.Second
const jobProcessTimeout = 40 * time.Second

type Worker struct {
	store *store.FileStore
	gsn   *gsn.Service
}

func New(store *store.FileStore, gsnService *gsn.Service) *Worker {
	return &Worker{
		store: store,
		gsn:   gsnService,
	}
}

func (w *Worker) Run(ctx context.Context, workerID string) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		job, ok, err := w.store.ClaimEstimateCalcJob(ctx, workerID, defaultLease)
		if err != nil {
			slog.Warn("estimate calc job claim failed", "worker", workerID, "error", err)
			sleep(ctx, 2*time.Second)
			continue
		}
		if !ok {
			sleep(ctx, time.Second)
			continue
		}

		if err := w.processJob(ctx, job); err != nil {
			slog.Warn("estimate calc job failed", "worker", workerID, "job", job.ID, "estimate", job.EstimateID, "line", job.LineID, "error", err)
			if failErr := w.store.FailEstimateCalcJob(ctx, job, err); failErr != nil {
				slog.Warn("estimate calc job fail update failed", "worker", workerID, "job", job.ID, "error", failErr)
			}
		}
	}
}

func (w *Worker) processJob(ctx context.Context, job store.EstimateCalcJob) error {
	started := time.Now()
	logCtx := observability.WithRequestID(ctx, job.RequestID)
	defer func() {
		observability.ObserveCalcDuration(time.Since(started).Seconds())
	}()

	code := job.Code
	if code == "" {
		observability.RecordCalcError("validate")
		return fmt.Errorf("record code is empty")
	}

	jobCtx, cancel := context.WithTimeout(logCtx, jobProcessTimeout)
	defer cancel()

	gsnStarted := time.Now()
	record, err := w.gsn.GetRecordDetail(jobCtx, code, job.FgisSetID, job.District)
	observability.ObserveCalcGSNDuration(time.Since(gsnStarted).Seconds())
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(jobCtx.Err(), context.DeadlineExceeded) {
			observability.RecordCalcError("gsn_timeout")
			return fmt.Errorf("GSN lookup timed out after %s", jobProcessTimeout)
		}
		if errors.Is(err, gsn.ErrNotConfigured) {
			observability.RecordCalcError("gsn_config")
			return err
		}
		observability.RecordCalcError("gsn")
		return fmt.Errorf("read GSN record %q: %w", code, err)
	}

	quantity, err := w.resolveJobQuantity(jobCtx, job)
	if err != nil {
		observability.RecordCalcError("quantity")
		return fmt.Errorf("resolve line quantity: %w", err)
	}

	pricingStarted := time.Now()
	pricing, err := estimatecalc.LinePricingFromRecord(record, quantity)
	observability.ObserveCalcPricingDuration(time.Since(pricingStarted).Seconds())
	if err != nil {
		observability.RecordCalcError("pricing")
		return fmt.Errorf("price record %q: %w", code, err)
	}

	calcJSON, err := json.Marshal(map[string]any{
		"record":    record,
		"quantity":  pricing.Quantity,
		"unitPrice": pricing.UnitPrice,
		"total":     pricing.Total,
	})
	if err != nil {
		return fmt.Errorf("encode calc result: %w", err)
	}

	return w.store.CompleteEstimateCalcJob(logCtx, job, store.EstimateLineCalcResult{
		Code:         record.Code,
		OriginalCode: record.OriginalCode,
		Name:         record.Name,
		Unit:         record.Unit,
		Quantity:     pricing.Quantity,
		UnitPrice:    pricing.UnitPrice,
		Total:        pricing.Total,
		CalcJSON:     calcJSON,
	})
}

func (w *Worker) resolveJobQuantity(ctx context.Context, job store.EstimateCalcJob) (float64, error) {
	storedQty := job.Quantity
	rawText := job.RawText
	if storedQty <= 0 && strings.TrimSpace(rawText) == "" {
		lineQty, lineRaw, err := w.store.EstimateLineQuantityContext(ctx, job.EstimateID, job.LineID)
		if err == nil {
			if storedQty <= 0 {
				storedQty = lineQty
			}
			if strings.TrimSpace(rawText) == "" {
				rawText = lineRaw
			}
		}
	}
	return store.ResolveEstimateLineQuantity(storedQty, rawText)
}

type Manager struct {
	store *store.FileStore
	gsn   *gsn.Service
}

func NewManager(store *store.FileStore, gsnService *gsn.Service) *Manager {
	return &Manager{
		store: store,
		gsn:   gsnService,
	}
}

func (m *Manager) Run(ctx context.Context) {
	if !m.gsn.Configured() {
		slog.Warn("GSN database is not configured; calc worker service is idle")
		<-ctx.Done()
		return
	}

	settings, err := m.store.GetAppSettings()
	if err != nil {
		slog.Warn("failed to read app settings", "error", err)
	} else {
		workerCount := store.NormalizeCalcWorkerCount(settings.CalcWorkerCount)
		m.gsn.SetMaxOpenConns(workerCount + 4)
	}

	if released, err := m.store.ReleaseStuckCalcJobLeases(ctx); err != nil {
		slog.Warn("failed to release stuck calc job leases", "error", err)
	} else if released > 0 {
		slog.Info("released stuck calc job leases", "count", released)
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
		if nextCount == currentCount {
			return
		}
		if cancel != nil {
			cancel()
		}
		var groupCtx context.Context
		groupCtx, cancel = context.WithCancel(ctx)
		startWorkerGroup(groupCtx, m.store, m.gsn, nextCount)
		slog.Info("calc worker group configured", "workers", nextCount)
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

func startWorkerGroup(ctx context.Context, fileStore *store.FileStore, gsnService *gsn.Service, count int) {
	worker := New(fileStore, gsnService)
	var wg sync.WaitGroup
	for i := 0; i < count; i += 1 {
		workerID := fmt.Sprintf("calc-worker-%d", i+1)
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker.Run(ctx, workerID)
		}()
	}
	go func() {
		<-ctx.Done()
		wg.Wait()
	}()
}

func sleep(ctx context.Context, delay time.Duration) {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
