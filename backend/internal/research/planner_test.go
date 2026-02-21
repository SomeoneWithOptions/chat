package research

import (
	"context"
	"errors"
	"testing"
)

type responderStub struct {
	response string
	err      error
}

func (s responderStub) Respond(_ context.Context, _ string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.response, nil
}

func TestJSONPlannerValidJSONPath(t *testing.T) {
	planner := NewJSONPlanner(responderStub{response: `{"nextAction":"search_more","queries":["golang release notes"],"coverageGaps":["official changelog"],"targetSourceTypes":["official docs"],"confidence":0.61,"reason":"need dated sources"}`})

	decision, err := planner.InitialPlan(context.Background(), PlannerInput{Question: "latest go release", MaxQueries: 4})
	if err != nil {
		t.Fatalf("initial plan: %v", err)
	}
	if decision.NextAction != NextActionSearchMore {
		t.Fatalf("expected search_more, got %s", decision.NextAction)
	}
	if len(decision.Queries) != 1 || decision.Queries[0] != "golang release notes" {
		t.Fatalf("unexpected queries: %+v", decision.Queries)
	}
}

func TestJSONPlannerMalformedJSONFallback(t *testing.T) {
	planner := NewJSONPlanner(responderStub{response: `not-json`})
	decision, err := planner.InitialPlan(context.Background(), PlannerInput{Question: "what changed", MaxQueries: 4})
	if err != nil {
		t.Fatalf("initial plan: %v", err)
	}
	if decision.NextAction != NextActionSearchMore {
		t.Fatalf("expected fallback search_more, got %s", decision.NextAction)
	}
	if len(decision.Queries) == 0 {
		t.Fatalf("expected fallback queries")
	}
}

func TestJSONPlannerEmptyQueryFallback(t *testing.T) {
	planner := NewJSONPlanner(responderStub{response: `{"nextAction":"search_more","queries":[],"coverageGaps":[],"targetSourceTypes":[],"confidence":0.2,"reason":""}`})
	decision, err := planner.InitialPlan(context.Background(), PlannerInput{Question: "question", MaxQueries: 2})
	if err != nil {
		t.Fatalf("initial plan: %v", err)
	}
	if decision.NextAction != NextActionSearchMore {
		t.Fatalf("expected search_more, got %s", decision.NextAction)
	}
	if len(decision.Queries) == 0 {
		t.Fatalf("expected fallback query list")
	}
}

func TestJSONPlannerFinalizeHonored(t *testing.T) {
	planner := NewJSONPlanner(responderStub{response: `{"nextAction":"finalize","queries":[],"coverageGaps":[],"targetSourceTypes":[],"confidence":0.91,"reason":"enough evidence"}`})
	decision, err := planner.EvaluateEvidence(context.Background(), PlannerInput{Question: "q"})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if decision.NextAction != NextActionFinalize {
		t.Fatalf("expected finalize action, got %s", decision.NextAction)
	}
}

func TestJSONPlannerResponderErrorFallback(t *testing.T) {
	planner := NewJSONPlanner(responderStub{err: errors.New("upstream unavailable")})
	decision, err := planner.EvaluateEvidence(context.Background(), PlannerInput{Question: "q", MaxLoops: 3, Loop: 3})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if decision.NextAction != NextActionFinalize {
		t.Fatalf("expected fallback finalize at budget limit, got %s", decision.NextAction)
	}
}
