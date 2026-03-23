PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  google_sub TEXT NOT NULL UNIQUE,
  email TEXT NOT NULL UNIQUE,
  display_name TEXT,
  avatar_url TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);

CREATE TABLE IF NOT EXISTS models (
  id TEXT PRIMARY KEY,
  provider TEXT NOT NULL,
  display_name TEXT NOT NULL,
  context_window INTEGER NOT NULL DEFAULT 0,
  prompt_price_microusd INTEGER NOT NULL DEFAULT 0,
  completion_price_microusd INTEGER NOT NULL DEFAULT 0,
  supported_parameters_json TEXT,
  supports_reasoning INTEGER NOT NULL DEFAULT 0,
  curated INTEGER NOT NULL DEFAULT 0,
  is_active INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_models_curated ON models(curated);

CREATE TABLE IF NOT EXISTS user_model_preferences (
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

CREATE TABLE IF NOT EXISTS user_model_favorites (
  user_id TEXT NOT NULL,
  model_id TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, model_id),
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  FOREIGN KEY (model_id) REFERENCES models(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS user_model_reasoning_presets (
  user_id TEXT NOT NULL,
  model_id TEXT NOT NULL,
  mode TEXT NOT NULL CHECK (mode IN ('chat', 'deep_research', 'fusion')),
  effort TEXT NOT NULL CHECK (effort IN ('low', 'medium', 'high')),
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, model_id, mode),
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  FOREIGN KEY (model_id) REFERENCES models(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS conversations (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  title TEXT NOT NULL DEFAULT 'New Chat',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_conversations_user_updated ON conversations(user_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS messages (
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

CREATE INDEX IF NOT EXISTS idx_messages_conversation_created ON messages(conversation_id, created_at);

CREATE TABLE IF NOT EXISTS citations (
  id TEXT PRIMARY KEY,
  message_id TEXT NOT NULL,
  url TEXT NOT NULL,
  title TEXT,
  snippet TEXT,
  source_provider TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (message_id) REFERENCES messages(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_citations_message_id ON citations(message_id);

CREATE TABLE IF NOT EXISTS files (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  filename TEXT NOT NULL,
  media_type TEXT NOT NULL,
  size_bytes INTEGER NOT NULL,
  storage_backend TEXT NOT NULL CHECK (storage_backend IN ('local', 'gcs')),
  storage_path TEXT NOT NULL,
  extracted_text TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_files_user_created ON files(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS message_files (
  message_id TEXT NOT NULL,
  file_id TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (message_id, file_id),
  FOREIGN KEY (message_id) REFERENCES messages(id) ON DELETE CASCADE,
  FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_message_files_file_id ON message_files(file_id);

CREATE TABLE IF NOT EXISTS fusion_runs (
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

CREATE INDEX IF NOT EXISTS idx_fusion_runs_status_created ON fusion_runs(status, created_at);
CREATE INDEX IF NOT EXISTS idx_fusion_runs_user_created ON fusion_runs(user_id, created_at DESC);

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
