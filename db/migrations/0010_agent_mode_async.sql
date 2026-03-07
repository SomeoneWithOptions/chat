ALTER TABLE user_model_preferences ADD COLUMN last_used_agent_model_id TEXT REFERENCES models(id) ON DELETE SET NULL;

ALTER TABLE messages ADD COLUMN response_mode TEXT NOT NULL DEFAULT 'chat' CHECK (response_mode IN ('chat', 'deep_research', 'agent'));
ALTER TABLE messages ADD COLUMN agent_summaries_json TEXT;

CREATE TABLE IF NOT EXISTS agent_runs (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  conversation_id TEXT NOT NULL,
  user_message_id TEXT NOT NULL,
  assistant_message_id TEXT NOT NULL,
  model_id TEXT NOT NULL,
  reasoning_effort TEXT,
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

CREATE INDEX IF NOT EXISTS idx_agent_runs_status_created ON agent_runs(status, created_at);
CREATE INDEX IF NOT EXISTS idx_agent_runs_user_created ON agent_runs(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS brave_monthly_usage (
  provider TEXT NOT NULL,
  month_key TEXT NOT NULL,
  queries_used INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (provider, month_key)
);

CREATE TABLE IF NOT EXISTS brave_rate_limits (
  provider TEXT PRIMARY KEY,
  next_allowed_at TEXT,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

PRAGMA foreign_keys = OFF;

CREATE TABLE user_model_reasoning_presets__new (
  user_id TEXT NOT NULL,
  model_id TEXT NOT NULL,
  mode TEXT NOT NULL CHECK (mode IN ('chat', 'deep_research', 'agent')),
  effort TEXT NOT NULL CHECK (effort IN ('low', 'medium', 'high')),
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, model_id, mode),
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  FOREIGN KEY (model_id) REFERENCES models(id) ON DELETE CASCADE
);

INSERT INTO user_model_reasoning_presets__new (user_id, model_id, mode, effort, updated_at)
SELECT user_id, model_id, mode, effort, updated_at
FROM user_model_reasoning_presets;

DROP TABLE user_model_reasoning_presets;
ALTER TABLE user_model_reasoning_presets__new RENAME TO user_model_reasoning_presets;

PRAGMA foreign_keys = ON;
