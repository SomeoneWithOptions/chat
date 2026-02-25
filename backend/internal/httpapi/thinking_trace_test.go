package httpapi

import (
	"fmt"
	"testing"

	"chat/backend/internal/research"
)

func TestThinkingTraceCollectorCapsEntries(t *testing.T) {
	collector := newThinkingTraceCollector()

	for i := 1; i <= maxThinkingTraceEntries+15; i++ {
		collector.AppendProgress(research.Progress{
			Phase: research.PhaseSearching,
			Title: fmt.Sprintf("Step %d", i),
		})
	}

	collector.MarkDone()
	snapshot := collector.Snapshot()
	if snapshot == nil {
		t.Fatal("expected snapshot")
	}
	if len(snapshot.Entries) != maxThinkingTraceEntries {
		t.Fatalf("expected %d entries, got %d", maxThinkingTraceEntries, len(snapshot.Entries))
	}
	if snapshot.Entries[0].Title != "Step 16" {
		t.Fatalf("expected first retained entry to be Step 16, got %q", snapshot.Entries[0].Title)
	}
	if snapshot.Status != thinkingTraceStatusDone {
		t.Fatalf("expected done status, got %q", snapshot.Status)
	}
}

func TestDecodeThinkingTraceJSONInvalidIsIgnored(t *testing.T) {
	if trace, ok := decodeThinkingTraceJSON("{not-json}"); ok || trace != nil {
		t.Fatalf("expected invalid JSON to be ignored, got trace=%+v ok=%t", trace, ok)
	}
}
