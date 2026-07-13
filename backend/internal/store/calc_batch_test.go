package store

import "testing"

func TestBuildEstimateCalcBatchResponse(t *testing.T) {
	lines := []EstimateCalcStatus{
		{LineID: "l1", Status: "done", Total: 10},
		{LineID: "l2", Status: "done", Total: 20},
		{LineID: "l3", Status: "queued"},
		{LineID: "l4", Status: "failed"},
	}

	resp, ready := buildEstimateCalcBatchResponse(7, 0, 2, lines, false)
	if !ready {
		t.Fatal("expected batch to be ready")
	}
	if resp.Generation != 7 {
		t.Fatalf("generation = %d", resp.Generation)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(resp.Items))
	}
	if resp.Applied != 2 {
		t.Fatalf("applied = %d, want 2", resp.Applied)
	}
	if resp.Processed != 3 {
		t.Fatalf("processed = %d, want 3", resp.Processed)
	}
	if resp.Errors != 1 {
		t.Fatalf("errors = %d, want 1", resp.Errors)
	}
	if resp.GrandTotal != 30 {
		t.Fatalf("grandTotal = %v, want 30", resp.GrandTotal)
	}

	resp, ready = buildEstimateCalcBatchResponse(7, 2, 10, []EstimateCalcStatus{
		{LineID: "l1", Status: "done", Total: 10},
		{LineID: "l2", Status: "done", Total: 20},
		{LineID: "l3", Status: "failed"},
	}, false)
	if !ready {
		t.Fatal("expected trailing batch to be ready at end of estimate")
	}
	if len(resp.Items) != 1 {
		t.Fatalf("trailing items = %d, want 1", len(resp.Items))
	}
	if resp.Applied != 3 {
		t.Fatalf("trailing applied = %d, want 3", resp.Applied)
	}

	_, ready = buildEstimateCalcBatchResponse(7, 3, 10, []EstimateCalcStatus{
		{LineID: "l1", Status: "done", Total: 10},
		{LineID: "l2", Status: "done", Total: 20},
		{LineID: "l3", Status: "failed"},
	}, false)
	if !ready {
		t.Fatal("expected completed batch to be ready")
	}

	resp, ready = buildEstimateCalcBatchResponse(7, 1, 2, []EstimateCalcStatus{
		{LineID: "l1", Status: "done"},
		{LineID: "l2", Status: "leased"},
		{LineID: "l3", Status: "queued"},
	}, false)
	if ready {
		t.Fatal("expected to wait until batch size is reached")
	}
	if len(resp.Items) != 0 {
		t.Fatalf("items = %d, want 0 while waiting", len(resp.Items))
	}
}

func TestCalcBatchReadyOrIdle(t *testing.T) {
	if !calcBatchReadyOrIdle(true, true) || !calcBatchReadyOrIdle(true, false) {
		t.Fatal("expected ready batches to return immediately")
	}
	if calcBatchReadyOrIdle(false, true) {
		t.Fatal("expected active calc to keep waiting")
	}
	if !calcBatchReadyOrIdle(false, false) {
		t.Fatal("expected idle calc to return without waiting")
	}
}

func TestBuildEstimateCalcBatchResponseWaitsUntilReady(t *testing.T) {
	lines := []EstimateCalcStatus{
		{LineID: "l1", Status: "done"},
		{LineID: "l2", Status: "leased"},
	}
	resp, ready := buildEstimateCalcBatchResponse(1, 1, 10, lines, false)
	if ready {
		t.Fatal("expected batch to wait for more terminal lines")
	}
	if len(resp.Items) != 0 {
		t.Fatalf("items = %d, want 0", len(resp.Items))
	}
}
