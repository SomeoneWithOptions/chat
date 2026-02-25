package httpapi

import (
	"encoding/json"
	"fmt"
	"strings"

	"chat/backend/internal/research"
)

const (
	thinkingTraceStatusRunning = "running"
	thinkingTraceStatusDone    = "done"
	thinkingTraceStatusStopped = "stopped"
	maxThinkingTraceEntries    = 60
)

type thinkingTraceEntry struct {
	Phase             research.Phase            `json:"phase"`
	Title             string                    `json:"title"`
	Detail            string                    `json:"detail,omitempty"`
	IsQuickStep       bool                      `json:"isQuickStep,omitempty"`
	Decision          research.ProgressDecision `json:"decision,omitempty"`
	Pass              *int                      `json:"pass,omitempty"`
	TotalPasses       *int                      `json:"totalPasses,omitempty"`
	Loop              *int                      `json:"loop,omitempty"`
	MaxLoops          *int                      `json:"maxLoops,omitempty"`
	SourcesConsidered *int                      `json:"sourcesConsidered,omitempty"`
	SourcesRead       *int                      `json:"sourcesRead,omitempty"`
}

type thinkingTrace struct {
	Status  string               `json:"status"`
	Summary string               `json:"summary"`
	Entries []thinkingTraceEntry `json:"entries"`
}

type thinkingTraceCollector struct {
	trace thinkingTrace
}

func newThinkingTraceCollector() *thinkingTraceCollector {
	return &thinkingTraceCollector{
		trace: thinkingTrace{
			Status:  thinkingTraceStatusRunning,
			Summary: "Working on your request",
			Entries: make([]thinkingTraceEntry, 0, 8),
		},
	}
}

func (c *thinkingTraceCollector) AppendProgress(progress research.Progress) {
	if c == nil {
		return
	}

	title := strings.TrimSpace(progress.Title)
	if title == "" {
		title = strings.TrimSpace(progress.Message)
	}
	if title == "" {
		title = "Working on your request"
	}

	detail := strings.TrimSpace(progress.Detail)
	entry := thinkingTraceEntry{
		Phase:             progress.Phase,
		Title:             title,
		Detail:            detail,
		IsQuickStep:       progress.IsQuickStep,
		Decision:          progress.Decision,
		Pass:              optionalPositiveInt(progress.Pass),
		TotalPasses:       optionalPositiveInt(progress.TotalPasses),
		Loop:              optionalPositiveInt(progress.Loop),
		MaxLoops:          optionalPositiveInt(progress.MaxLoops),
		SourcesConsidered: optionalNonNegativeInt(progress.SourcesConsidered),
		SourcesRead:       optionalNonNegativeInt(progress.SourcesRead),
	}

	c.trace.Entries = append(c.trace.Entries, entry)
	if len(c.trace.Entries) > maxThinkingTraceEntries {
		c.trace.Entries = c.trace.Entries[len(c.trace.Entries)-maxThinkingTraceEntries:]
	}

	if detail != "" {
		c.trace.Summary = fmt.Sprintf("%s: %s", title, detail)
		return
	}
	c.trace.Summary = title
}

func (c *thinkingTraceCollector) MarkDone() {
	if c == nil {
		return
	}
	c.trace.Status = thinkingTraceStatusDone
	if strings.TrimSpace(c.trace.Summary) == "" {
		c.trace.Summary = "Thought process complete"
	}
}

func (c *thinkingTraceCollector) MarkStopped(summary string) {
	if c == nil {
		return
	}
	c.trace.Status = thinkingTraceStatusStopped
	if trimmed := strings.TrimSpace(summary); trimmed != "" {
		c.trace.Summary = trimmed
		return
	}
	if strings.TrimSpace(c.trace.Summary) == "" {
		c.trace.Summary = "Stopped"
	}
}

func (c *thinkingTraceCollector) Snapshot() *thinkingTrace {
	if c == nil {
		return nil
	}
	if len(c.trace.Entries) == 0 {
		return nil
	}
	entries := make([]thinkingTraceEntry, len(c.trace.Entries))
	copy(entries, c.trace.Entries)
	return &thinkingTrace{
		Status:  c.trace.Status,
		Summary: c.trace.Summary,
		Entries: entries,
	}
}

func encodeThinkingTraceJSON(trace *thinkingTrace) (any, error) {
	if trace == nil {
		return nil, nil
	}
	if len(trace.Entries) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(trace)
	if err != nil {
		return nil, err
	}
	return string(encoded), nil
}

func decodeThinkingTraceJSON(raw string) (*thinkingTrace, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, false
	}

	var parsed thinkingTrace
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return nil, false
	}
	if len(parsed.Entries) == 0 {
		return nil, false
	}
	status := strings.TrimSpace(parsed.Status)
	switch status {
	case thinkingTraceStatusRunning, thinkingTraceStatusDone, thinkingTraceStatusStopped:
		// valid
	default:
		parsed.Status = thinkingTraceStatusDone
	}
	if strings.TrimSpace(parsed.Summary) == "" {
		last := parsed.Entries[len(parsed.Entries)-1]
		if strings.TrimSpace(last.Detail) != "" {
			parsed.Summary = fmt.Sprintf("%s: %s", last.Title, last.Detail)
		} else {
			parsed.Summary = last.Title
		}
	}
	return &parsed, true
}

func optionalPositiveInt(value int) *int {
	if value <= 0 {
		return nil
	}
	v := value
	return &v
}

func optionalNonNegativeInt(value int) *int {
	if value < 0 {
		return nil
	}
	v := value
	return &v
}
