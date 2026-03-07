package research

import "time"

type ModeProfile string

const (
	ModeChat         ModeProfile = "chat"
	ModeDeepResearch ModeProfile = "deep_research"
	ModeAgent        ModeProfile = "agent"
)

const (
	defaultChatMaxLoops                     = 2
	defaultChatMaxSourcesRead               = 4
	defaultChatMaxSearchQueries             = 4
	defaultChatMaxCitations                 = 8
	defaultChatTimeout                      = 20 * time.Second
	defaultDeepMaxLoops                     = 6
	defaultDeepMaxSourcesRead               = 16
	defaultDeepMaxSearchQueries             = 18
	defaultDeepMaxCitations                 = 12
	defaultDeepTimeout                      = 150 * time.Second
	defaultAgentMinSearchQueries            = 20
	defaultAgentMaxLoops                    = 12
	defaultAgentMaxSourcesRead              = 80
	defaultAgentMaxSearchQueries            = 60
	defaultAgentMaxCitations                = 16
	defaultAgentTimeout                     = 20 * time.Minute
	defaultAgentSearchResultsPerQuery       = 10
	defaultSearchResultsPerQuery            = 6
	defaultSourceFetchTimeout               = 12 * time.Second
	defaultSourceMaxBytes             int64 = 1_500_000
)

func DefaultProfile(mode ModeProfile) OrchestratorConfig {
	switch mode {
	case ModeAgent:
		return OrchestratorConfig{
			MinSearchQueries:   defaultAgentMinSearchQueries,
			MaxLoops:           defaultAgentMaxLoops,
			MaxSourcesRead:     defaultAgentMaxSourcesRead,
			MaxSearchQueries:   defaultAgentMaxSearchQueries,
			MaxCitations:       defaultAgentMaxCitations,
			SearchResultsPerQ:  defaultAgentSearchResultsPerQuery,
			Timeout:            defaultAgentTimeout,
			MinSearchInterval:  defaultRateLimitRetryDelay,
			SourceFetchTimeout: defaultSourceFetchTimeout,
			SourceMaxBytes:     defaultSourceMaxBytes,
		}
	case ModeDeepResearch:
		return OrchestratorConfig{
			MaxLoops:           defaultDeepMaxLoops,
			MaxSourcesRead:     defaultDeepMaxSourcesRead,
			MaxSearchQueries:   defaultDeepMaxSearchQueries,
			MaxCitations:       defaultDeepMaxCitations,
			SearchResultsPerQ:  defaultSearchResultsPerQuery,
			Timeout:            defaultDeepTimeout,
			MinSearchInterval:  defaultRateLimitRetryDelay,
			SourceFetchTimeout: defaultSourceFetchTimeout,
			SourceMaxBytes:     defaultSourceMaxBytes,
		}
	default:
		return OrchestratorConfig{
			MaxLoops:           defaultChatMaxLoops,
			MaxSourcesRead:     defaultChatMaxSourcesRead,
			MaxSearchQueries:   defaultChatMaxSearchQueries,
			MaxCitations:       defaultChatMaxCitations,
			SearchResultsPerQ:  defaultSearchResultsPerQuery,
			Timeout:            defaultChatTimeout,
			MinSearchInterval:  0,
			SourceFetchTimeout: defaultSourceFetchTimeout,
			SourceMaxBytes:     defaultSourceMaxBytes,
		}
	}
}

func ResolveProfile(mode ModeProfile, overrides OrchestratorConfig) OrchestratorConfig {
	resolved := DefaultProfile(mode)

	if overrides.MaxLoops > 0 {
		resolved.MaxLoops = overrides.MaxLoops
	}
	if overrides.MinSearchQueries > 0 {
		resolved.MinSearchQueries = overrides.MinSearchQueries
	}
	if overrides.MaxSourcesRead > 0 {
		resolved.MaxSourcesRead = overrides.MaxSourcesRead
	}
	if overrides.MaxSearchQueries > 0 {
		resolved.MaxSearchQueries = overrides.MaxSearchQueries
	}
	if overrides.MaxCitations > 0 {
		resolved.MaxCitations = overrides.MaxCitations
	}
	if overrides.SearchResultsPerQ > 0 {
		resolved.SearchResultsPerQ = overrides.SearchResultsPerQ
	}
	if overrides.Timeout > 0 {
		resolved.Timeout = overrides.Timeout
	}
	if overrides.MinSearchInterval >= 0 {
		resolved.MinSearchInterval = overrides.MinSearchInterval
	}
	if overrides.SourceFetchTimeout > 0 {
		resolved.SourceFetchTimeout = overrides.SourceFetchTimeout
	}
	if overrides.SourceMaxBytes > 0 {
		resolved.SourceMaxBytes = overrides.SourceMaxBytes
	}

	if resolved.MaxLoops < 1 {
		resolved.MaxLoops = 1
	}
	if resolved.MinSearchQueries < 0 {
		resolved.MinSearchQueries = 0
	}
	if resolved.MaxSourcesRead < 1 {
		resolved.MaxSourcesRead = 1
	}
	if resolved.MaxSearchQueries < 1 {
		resolved.MaxSearchQueries = 1
	}
	if resolved.MaxCitations < 1 {
		resolved.MaxCitations = 1
	}
	if resolved.SearchResultsPerQ < 1 {
		resolved.SearchResultsPerQ = defaultSearchResultsPerQuery
	}
	if resolved.SourceFetchTimeout <= 0 {
		resolved.SourceFetchTimeout = defaultSourceFetchTimeout
	}
	if resolved.SourceMaxBytes <= 0 {
		resolved.SourceMaxBytes = defaultSourceMaxBytes
	}
	if resolved.MaxSearchQueries < resolved.MinSearchQueries {
		resolved.MaxSearchQueries = resolved.MinSearchQueries
	}

	return resolved
}
