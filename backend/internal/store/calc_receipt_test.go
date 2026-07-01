package store

import "testing"

func TestCalcDeliverySkipReason(t *testing.T) {
	tests := []struct {
		name         string
		receiptCount int
		lineRevision int64
		lineStatus   string
		jobRevision  int64
		wantSkip     bool
		wantReason   string
	}{
		{name: "receipt on queued line", receiptCount: 1, lineRevision: 2, lineStatus: "queued", jobRevision: 2, wantSkip: false},
		{name: "stale revision", lineRevision: 3, lineStatus: "queued", jobRevision: 2, wantSkip: true, wantReason: "stale line revision"},
		{name: "already done", lineRevision: 2, lineStatus: "done", jobRevision: 2, wantSkip: true, wantReason: "line already done"},
		{name: "process", lineRevision: 2, lineStatus: "queued", jobRevision: 2, wantSkip: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			skip, reason := calcDeliverySkipReason(tc.receiptCount, tc.lineRevision, tc.lineStatus, tc.jobRevision)
			if skip != tc.wantSkip || reason != tc.wantReason {
				t.Fatalf("got skip=%v reason=%q want skip=%v reason=%q", skip, reason, tc.wantSkip, tc.wantReason)
			}
		})
	}
}
