PRAGMA foreign_keys = OFF;

CREATE TABLE user_model_preferences__new (
  user_id TEXT PRIMARY KEY,
  last_used_model_id TEXT,
  last_used_deep_research_model_id TEXT,
  last_used_fusion_mode_model_id TEXT,
  last_used_fusion_source_model_ids_json TEXT,
  last_used_fusion_model_id TEXT,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  FOREIGN KEY (last_used_model_id) REFERENCES models(id) ON DELETE SET NULL,
  FOREIGN KEY (last_used_deep_research_model_id) REFERENCES models(id) ON DELETE SET NULL,
  FOREIGN KEY (last_used_fusion_mode_model_id) REFERENCES models(id) ON DELETE SET NULL
);

INSERT INTO user_model_preferences__new (
  user_id,
  last_used_model_id,
  last_used_deep_research_model_id,
  last_used_fusion_mode_model_id,
  last_used_fusion_source_model_ids_json,
  last_used_fusion_model_id,
  updated_at
)
SELECT
  user_id,
  last_used_model_id,
  last_used_deep_research_model_id,
  last_used_agent_model_id,
  last_used_agent_source_model_ids_json,
  last_used_agent_fusion_model_id,
  updated_at
FROM user_model_preferences;

DROP TABLE user_model_preferences;
ALTER TABLE user_model_preferences__new RENAME TO user_model_preferences;

CREATE TABLE user_model_reasoning_presets__new (
  user_id TEXT NOT NULL,
  model_id TEXT NOT NULL,
  mode TEXT NOT NULL CHECK (mode IN ('chat', 'deep_research', 'fusion')),
  effort TEXT NOT NULL CHECK (effort IN ('low', 'medium', 'high')),
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, model_id, mode),
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  FOREIGN KEY (model_id) REFERENCES models(id) ON DELETE CASCADE
);

INSERT INTO user_model_reasoning_presets__new (user_id, model_id, mode, effort, updated_at)
SELECT
  user_id,
  model_id,
  CASE WHEN mode = 'agent' THEN 'fusion' ELSE mode END,
  effort,
  updated_at
FROM user_model_reasoning_presets;

DROP TABLE user_model_reasoning_presets;
ALTER TABLE user_model_reasoning_presets__new RENAME TO user_model_reasoning_presets;

CREATE TABLE messages__new (
  id TEXT PRIMARY KEY,
  conversation_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  role TEXT NOT NULL CHECK (role IN ('system', 'user', 'assistant', 'tool')),
  content TEXT NOT NULL,
  reasoning_content TEXT,
  thinking_trace_json TEXT,
  model_id TEXT,
  prompt_tokens INTEGER,
  completion_tokens INTEGER,
  total_tokens INTEGER,
  reasoning_tokens INTEGER,
  cost_microusd INTEGER,
  byok_inference_cost_microusd INTEGER,
  tokens_per_second REAL,
  usage_model_id TEXT,
  usage_provider_name TEXT,
  grounding_enabled INTEGER NOT NULL DEFAULT 1,
  deep_research_enabled INTEGER NOT NULL DEFAULT 0,
  response_mode TEXT NOT NULL DEFAULT 'chat' CHECK (response_mode IN ('chat', 'deep_research', 'fusion')),
  fusion_summaries_json TEXT,
  fusion_sources_json TEXT,
  fusion_analysis_json TEXT,
  fusion_result_model_id TEXT,
  fusion_result_usage_json TEXT,
  fusion_run_id TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE,
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  FOREIGN KEY (model_id) REFERENCES models(id) ON DELETE SET NULL
);

INSERT INTO messages__new (
  id,
  conversation_id,
  user_id,
  role,
  content,
  reasoning_content,
  thinking_trace_json,
  model_id,
  prompt_tokens,
  completion_tokens,
  total_tokens,
  reasoning_tokens,
  cost_microusd,
  byok_inference_cost_microusd,
  tokens_per_second,
  usage_model_id,
  usage_provider_name,
  grounding_enabled,
  deep_research_enabled,
  response_mode,
  fusion_summaries_json,
  fusion_sources_json,
  fusion_analysis_json,
  fusion_result_model_id,
  fusion_result_usage_json,
  fusion_run_id,
  created_at
)
SELECT
  id,
  conversation_id,
  user_id,
  role,
  content,
  reasoning_content,
  thinking_trace_json,
  model_id,
  prompt_tokens,
  completion_tokens,
  total_tokens,
  reasoning_tokens,
  cost_microusd,
  byok_inference_cost_microusd,
  tokens_per_second,
  usage_model_id,
  usage_provider_name,
  grounding_enabled,
  deep_research_enabled,
  CASE WHEN response_mode = 'agent' THEN 'fusion' ELSE response_mode END,
  agent_summaries_json,
  agent_sources_json,
  agent_analysis_json,
  agent_result_model_id,
  agent_result_usage_json,
  agent_run_id,
  created_at
FROM messages;

DROP TABLE messages;
ALTER TABLE messages__new RENAME TO messages;
CREATE INDEX IF NOT EXISTS idx_messages_conversation_created ON messages(conversation_id, created_at);

CREATE TABLE fusion_runs (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  conversation_id TEXT NOT NULL,
  user_message_id TEXT NOT NULL,
  assistant_message_id TEXT NOT NULL,
  model_id TEXT NOT NULL,
  reasoning_effort TEXT,
  workflow_type TEXT NOT NULL DEFAULT 'single_model',
  source_model_ids_json TEXT,
  fusion_model_id TEXT,
  grounding_enabled INTEGER NOT NULL DEFAULT 1,
  fusion_config_json TEXT,
  source_results_json TEXT,
  fusion_analysis_json TEXT,
  fusion_result_json TEXT,
  completed_sources INTEGER NOT NULL DEFAULT 0,
  degraded_sources INTEGER NOT NULL DEFAULT 0,
  failed_sources INTEGER NOT NULL DEFAULT 0,
  public_status_json TEXT,
  status TEXT NOT NULL CHECK (status IN ('queued', 'running', 'completed', 'failed')),
  search_budget INTEGER NOT NULL DEFAULT 0,
  searches_used INTEGER NOT NULL DEFAULT 0,
  sources_read INTEGER NOT NULL DEFAULT 0,
  last_error TEXT,
  started_at TEXT,
  finished_at TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE,
  FOREIGN KEY (user_message_id) REFERENCES messages(id) ON DELETE CASCADE,
  FOREIGN KEY (assistant_message_id) REFERENCES messages(id) ON DELETE CASCADE
);

INSERT INTO fusion_runs (
  id,
  user_id,
  conversation_id,
  user_message_id,
  assistant_message_id,
  model_id,
  reasoning_effort,
  workflow_type,
  source_model_ids_json,
  fusion_model_id,
  grounding_enabled,
  fusion_config_json,
  source_results_json,
  fusion_analysis_json,
  fusion_result_json,
  completed_sources,
  degraded_sources,
  failed_sources,
  public_status_json,
  status,
  search_budget,
  searches_used,
  sources_read,
  last_error,
  started_at,
  finished_at,
  created_at,
  updated_at
)
SELECT
  id,
  user_id,
  conversation_id,
  user_message_id,
  assistant_message_id,
  model_id,
  reasoning_effort,
  CASE WHEN workflow_type = 'council_fusion' THEN 'multi_model' ELSE workflow_type END,
  source_model_ids_json,
  fusion_model_id,
  grounding_enabled,
  council_config_json,
  source_results_json,
  fusion_analysis_json,
  fusion_result_json,
  completed_sources,
  degraded_sources,
  failed_sources,
  public_status_json,
  status,
  search_budget,
  searches_used,
  sources_read,
  last_error,
  started_at,
  finished_at,
  created_at,
  updated_at
FROM agent_runs;

DROP TABLE agent_runs;
CREATE INDEX IF NOT EXISTS idx_fusion_runs_status_created ON fusion_runs(status, created_at);
CREATE INDEX IF NOT EXISTS idx_fusion_runs_user_created ON fusion_runs(user_id, created_at DESC);

PRAGMA foreign_keys = ON;
