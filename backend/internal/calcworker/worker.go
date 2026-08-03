package calcworker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
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

	if store.IsUserCatalogCipherCode(code) {
		return w.processUserCatalogJob(jobCtx, job, code)
	}

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

	quantity, rawText, err := w.resolveJobQuantity(jobCtx, job)
	if err != nil {
		observability.RecordCalcError("quantity")
		return fmt.Errorf("resolve line quantity: %w", err)
	}

	if replacements := store.SourceDataResourceReplacementsFromRawText(rawText); len(replacements) > 0 {
		gsnReplacements := make([]gsn.ResourceNumberReplacement, 0, len(replacements))
		for _, rep := range replacements {
			gsnReplacements = append(gsnReplacements, gsn.ResourceNumberReplacement{
				FromNumber:       rep.FromNumber,
				ToNumber:         rep.ToNumber,
				AbsoluteQuantity: rep.AbsoluteQuantity,
				Coefficient:      rep.Coefficient,
				HasCoefficient:   rep.HasCoefficient,
			})
		}
		replaced, replaceErr := w.gsn.ApplyResourceNumberReplacements(
			jobCtx, record.Resources, gsnReplacements, job.FgisSetID, job.District,
		)
		if replaceErr != nil {
			observability.RecordCalcError("resource_replace")
			return fmt.Errorf("apply resource replacements for %q: %w", code, replaceErr)
		}
		record.Resources = replaced
	}

	if deletions := store.SourceDataResourceDeletionsFromRawText(rawText); len(deletions) > 0 {
		numbers := make([]string, 0, len(deletions))
		for _, del := range deletions {
			numbers = append(numbers, del.Number)
		}
		record.Resources = gsn.ApplyResourceNumberDeletions(record.Resources, numbers)
	}

	pricingStarted := time.Now()
	snap, err := estimatecalc.BuildLineCalcSnapshot(record, quantity)
	observability.ObserveCalcPricingDuration(time.Since(pricingStarted).Seconds())
	if err != nil {
		observability.RecordCalcError("pricing")
		return fmt.Errorf("price record %q: %w", code, err)
	}
	estimatecalc.ApplyDeterminantAssignment(&snap, store.SourceDataDeterminantFromRawText(rawText))

	calcJSON, err := json.Marshal(map[string]any{
		"record":        record,
		"quantity":      snap.Quantity,
		"unitPrice":     snap.UnitPrice,
		"total":         snap.Total,
		"resourcesText": snap.ResourcesText,
	})
	if err != nil {
		return fmt.Errorf("encode calc result: %w", err)
	}

	return w.store.CompleteEstimateCalcJob(logCtx, job, store.EstimateLineCalcResult{
		Code:         record.Code,
		OriginalCode: record.OriginalCode,
		Name:         record.Name,
		Unit:         record.Unit,
		Quantity:     snap.Quantity,
		UnitPrice:    snap.UnitPrice,
		Total:        snap.Total,
		CalcJSON:     calcJSON,
		Snapshot:     &snap,
	})
}

func (w *Worker) processUserCatalogJob(ctx context.Context, job store.EstimateCalcJob, code string) error {
	quantity, rawText, err := w.resolveJobQuantity(ctx, job)
	if err != nil {
		observability.RecordCalcError("quantity")
		return fmt.Errorf("resolve line quantity: %w", err)
	}
	if strings.TrimSpace(rawText) == "" {
		rawText = job.RawText
	}

	fields, err := store.ParseSourceDataPositionFields(rawText)
	if err != nil {
		observability.RecordCalcError("user_catalog_parse")
		return fmt.Errorf("parse user catalog line %q: %w", code, err)
	}

	name := fields.Name
	unit := fields.Unit
	total := fields.Total
	unitPrice := 0.0

	if store.UserCatalogPositionNeedsLookup(fields) {
		position, found, err := w.store.LookupUserPositionByCode(ctx, job.CompanyID, code)
		if err != nil {
			observability.RecordCalcError("user_catalog")
			return fmt.Errorf("lookup user position %q: %w", code, err)
		}
		if !found {
			observability.RecordCalcError("user_catalog")
			return fmt.Errorf("user position %q not found", code)
		}
		if !fields.HasName {
			name = position.Name
		}
		if !fields.HasUnit {
			unit = position.Unit
		}
		if !fields.HasTotal {
			unitPrice = position.Cost
			total = unitPrice * quantity
		}
	}

	if fields.HasTotal && total > 0 && quantity > 0 {
		unitPrice = estimatecalc.RoundMoney(total / quantity)
	} else if unitPrice == 0 && total > 0 && quantity > 0 {
		unitPrice = estimatecalc.RoundMoney(total / quantity)
	} else if unitPrice > 0 {
		unitPrice = estimatecalc.RoundMoney(unitPrice)
	}

	determinant := store.SourceDataDeterminantFromRawText(rawText)
	snap := estimatecalc.LineCalcSnapshot{
		Code:         code,
		OriginalCode: "",
		Name:         name,
		Unit:         unit,
		Determinant:  determinant,
		Quantity:     quantity,
		UnitPrice:    unitPrice,
		Total:        total,
	}
	if determinant != "" {
		snap.Resources = []estimatecalc.ResourceContribution{{
			Code:          code,
			Determinant:   determinant,
			Consumption:   quantity,
			Name:          name,
			Unit:          unit,
			EstimatePrice: unitPrice,
		}}
		snap.ResourcesText = estimatecalc.FormatResourcesText(snap.Resources)
	}

	record := map[string]any{
		"code":           code,
		"originalCode":   "",
		"name":           name,
		"unit":           unit,
		"determinant":    determinant,
		"isWork":         false,
		"unitPriceText":  formatCalcMoneyText(unitPrice),
		"unitPriceIndex": "",
	}
	calcJSON, err := json.Marshal(map[string]any{
		"record":    record,
		"quantity":  quantity,
		"unitPrice": unitPrice,
		"total":     total,
	})
	if err != nil {
		return fmt.Errorf("encode user catalog calc result: %w", err)
	}

	return w.store.CompleteEstimateCalcJob(ctx, job, store.EstimateLineCalcResult{
		Code:         code,
		OriginalCode: "",
		Name:         name,
		Unit:         unit,
		Quantity:     quantity,
		UnitPrice:    unitPrice,
		Total:        total,
		CalcJSON:     calcJSON,
		Snapshot:     &snap,
	})
}

func formatCalcMoneyText(value float64) string {
	value = estimatecalc.RoundMoney(value)
	if value == 0 {
		return "0"
	}
	return strings.ReplaceAll(strconv.FormatFloat(value, 'f', 2, 64), ".", ",")
}

func (w *Worker) resolveJobQuantity(ctx context.Context, job store.EstimateCalcJob) (float64, string, error) {
	storedQty := job.Quantity
	rawText := job.RawText
	if storedQty <= 0 || strings.TrimSpace(rawText) == "" {
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
	quantity, err := store.ResolveEstimateLineQuantity(storedQty, rawText)
	return quantity, rawText, err
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
