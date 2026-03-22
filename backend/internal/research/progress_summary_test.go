package research

import "testing"

func TestBuildProgressSummaryUsesUserFacingCopy(t *testing.T) {
	summary := BuildProgressSummary(ProgressSummaryInput{Phase: PhasePlanning})
	if summary.Title != "Mapping the request" {
		t.Fatalf("expected planning title to be updated, got %q", summary.Title)
	}
	if summary.Detail != "Figuring out what needs to be answered before drafting anything." {
		t.Fatalf("expected planning detail to be updated, got %q", summary.Detail)
	}
}

func TestBuildProgressSummaryKeepsQuickSearchMinimal(t *testing.T) {
	summary := BuildProgressSummary(ProgressSummaryInput{
		Phase:      PhaseSearching,
		QueryCount: 1,
	})

	if summary.Title != "Finding an initial source" {
		t.Fatalf("expected quick search title, got %q", summary.Title)
	}
	if !summary.IsQuickStep {
		t.Fatal("expected quick search to stay marked as a quick step")
	}
	if summary.Detail != "" {
		t.Fatalf("expected quick search detail to be blank, got %q", summary.Detail)
	}
}
