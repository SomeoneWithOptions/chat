package httpapi

import (
	"testing"
	"time"

	"chat/backend/internal/config"
	"chat/backend/internal/research"
)

func TestBuildResearchConfigAppliesBraveSpacingToDeepResearch(t *testing.T) {
	h := Handler{
		cfg: config.Config{
			ChatResearchTimeoutSeconds: 20,
			DeepResearchTimeoutSeconds: 150,
		},
	}

	chatCfg := h.buildResearchConfig(research.ModeChat)
	if chatCfg.MinSearchInterval != 0 {
		t.Fatalf("expected chat min search interval to remain default 0, got %v", chatCfg.MinSearchInterval)
	}
	if chatCfg.Timeout != 20*time.Second {
		t.Fatalf("expected chat timeout 20s, got %v", chatCfg.Timeout)
	}

	deepCfg := h.buildResearchConfig(research.ModeDeepResearch)
	if deepCfg.MinSearchInterval != braveFreeTierSpacing {
		t.Fatalf("expected deep min search interval %v, got %v", braveFreeTierSpacing, deepCfg.MinSearchInterval)
	}
	if deepCfg.Timeout != 150*time.Second {
		t.Fatalf("expected deep timeout 150s, got %v", deepCfg.Timeout)
	}
}
