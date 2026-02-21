package research

import (
	"testing"
	"time"
)

func TestDefaultProfileValues(t *testing.T) {
	chat := DefaultProfile(ModeChat)
	if chat.MaxLoops != 2 || chat.MaxSourcesRead != 4 || chat.MaxSearchQueries != 4 || chat.MaxCitations != 8 {
		t.Fatalf("unexpected chat defaults: %+v", chat)
	}
	if chat.Timeout != 20*time.Second {
		t.Fatalf("unexpected chat timeout: %v", chat.Timeout)
	}

	deep := DefaultProfile(ModeDeepResearch)
	if deep.MaxLoops != 6 || deep.MaxSourcesRead != 16 || deep.MaxSearchQueries != 18 || deep.MaxCitations != 12 {
		t.Fatalf("unexpected deep defaults: %+v", deep)
	}
	if deep.Timeout != 150*time.Second {
		t.Fatalf("unexpected deep timeout: %v", deep.Timeout)
	}
}

func TestResolveProfileAppliesOverridesAndClamps(t *testing.T) {
	resolved := ResolveProfile(ModeChat, OrchestratorConfig{
		MaxLoops:           -1,
		MaxSourcesRead:     0,
		MaxSearchQueries:   -4,
		MaxCitations:       0,
		SearchResultsPerQ:  0,
		Timeout:            11 * time.Second,
		SourceFetchTimeout: -1,
		SourceMaxBytes:     0,
	})

	if resolved.MaxLoops != 2 || resolved.MaxSourcesRead != 4 || resolved.MaxSearchQueries != 4 || resolved.MaxCitations != 8 {
		t.Fatalf("expected invalid values to clamp to defaults: %+v", resolved)
	}
	if resolved.Timeout != 11*time.Second {
		t.Fatalf("expected timeout override to apply, got %v", resolved.Timeout)
	}
	if resolved.SourceFetchTimeout <= 0 || resolved.SourceMaxBytes <= 0 {
		t.Fatalf("expected source constraints to be positive: %+v", resolved)
	}
}
