package research

import (
	"context"
	"time"
)

type NextAction string

const (
	NextActionSearchMore NextAction = "search_more"
	NextActionFinalize   NextAction = "finalize"
)

type StopReason string

const (
	StopReasonSufficient      StopReason = "sufficient"
	StopReasonBudgetExhausted StopReason = "budget_exhausted"
	StopReasonTimeout         StopReason = "timeout"
	StopReasonError           StopReason = "error"
)

type OrchestratorConfig struct {
	MaxLoops           int
	MaxSourcesRead     int
	MaxSearchQueries   int
	MaxCitations       int
	SearchResultsPerQ  int
	Timeout            time.Duration
	MinSearchInterval  time.Duration
	SourceFetchTimeout time.Duration
	SourceMaxBytes     int64
}

type PlannerInput struct {
	Question             string
	TimeSensitive        bool
	Loop                 int
	MaxLoops             int
	UsedQueries          int
	MaxQueries           int
	RemainingReadBudget  int
	CoverageGaps         []string
	PreviousQueries      []string
	Evidence             []Evidence
	LatestReadCandidates []Citation
}

type PlannerDecision struct {
	NextAction        NextAction `json:"nextAction"`
	Queries           []string   `json:"queries"`
	CoverageGaps      []string   `json:"coverageGaps"`
	TargetSourceTypes []string   `json:"targetSourceTypes"`
	Confidence        float64    `json:"confidence"`
	Reason            string     `json:"reason"`
}

type Planner interface {
	InitialPlan(ctx context.Context, input PlannerInput) (PlannerDecision, error)
	EvaluateEvidence(ctx context.Context, input PlannerInput) (PlannerDecision, error)
}

type Reader interface {
	Read(ctx context.Context, rawURL string) (ReadResult, error)
}

type ReadResult struct {
	URL         string
	FinalURL    string
	Title       string
	ContentType string
	Text        string
	Snippet     string
	FetchStatus string
	FetchedAt   time.Time
	Truncated   bool
}

type Evidence struct {
	Citation
	CanonicalURL  string
	ContentType   string
	Excerpt       string
	SourceQuality float64
	Freshness     float64
	Completeness  float64
	Corroboration float64
	Contradiction bool
	FetchedAt     time.Time
	HasFullText   bool
}

type OrchestratorResult struct {
	Loops              int
	SearchQueries      int
	SourcesConsidered  int
	SourcesRead        int
	ReadAttempts       int
	ReadFailures       int
	ReadFailureReasons map[string]int
	Citations          []Citation
	Evidence           []Evidence
	Warnings           []string
	Warning            string
	StopReason         StopReason
}
