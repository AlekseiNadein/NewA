package observability

import "testing"

func TestCombinedSnapshotDoesNotPanic(t *testing.T) {
	snap, services := CombinedSnapshot("http://127.0.0.1:9091/metrics", "http://127.0.0.1:9092/metrics")
	if services["api"] != true {
		t.Fatalf("expected api service=true, got %#v", services)
	}
	if snap.RabbitConnected == nil {
		t.Fatal("expected rabbit connected map")
	}
}
