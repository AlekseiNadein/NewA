package store

import (
	"testing"

	"nav-saas-mvp/backend/internal/domain"
)

func TestPrepareEstimateLineForStoragePreservesCalcWhenRevisionUnchanged(t *testing.T) {
	estimate := domain.Estimate{
		ID:         "est_test",
		FgisSetID:  "alrosa-2026-q2",
		District:   "14.3",
	}
	line := domain.EstimateItem{
		ID:       "line_1",
		Type:     string(domain.EstimateLinePosition),
		Source:   "gsn",
		Code:     "E001",
		Quantity: 2,
	}
	revision := estimateLineRevision(estimate, line)
	existing := storedEstimateLineCalc{
		Revision:   revision,
		CalcStatus: "done",
		Total:      1500,
		UnitPrice:  750,
	}

	out := prepareEstimateLineForStorage(estimate, line, existing, true)
	if out.CalcStatus != "done" {
		t.Fatalf("calc status = %q, want done", out.CalcStatus)
	}
	if out.Total != 1500 {
		t.Fatalf("total = %v, want 1500", out.Total)
	}
}

func TestPrepareEstimateLineForStorageClearsCalcWhenRevisionChanges(t *testing.T) {
	estimate := domain.Estimate{
		ID:        "est_test",
		FgisSetID: "alrosa-2026-q2",
		District:  "14.11",
	}
	line := domain.EstimateItem{
		ID:       "line_1",
		Type:     string(domain.EstimateLinePosition),
		Source:   "gsn",
		Code:     "E001",
		Quantity: 2,
	}
	existing := storedEstimateLineCalc{
		Revision:   estimateLineRevision(domain.Estimate{ID: "est_test", FgisSetID: "alrosa-2026-q2", District: "14.3"}, line),
		CalcStatus: "done",
		Total:      1500,
	}

	out := prepareEstimateLineForStorage(estimate, line, existing, true)
	if out.CalcStatus != "" {
		t.Fatalf("calc status = %q, want empty", out.CalcStatus)
	}
	if out.Total != 0 {
		t.Fatalf("total = %v, want 0", out.Total)
	}
}
