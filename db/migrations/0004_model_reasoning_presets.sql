-- 0004_model_reasoning_presets.sql
-- Add model capability metadata and user reasoning-effort presets.

ALTER TABLE models ADD COLUMN supported_parameters_json TEXT;
ALTER TABLE models ADD COLUMN supports_reasoning INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS user_model_reasoning_presets (
  user_id TEXT NOT NULL,
  model_id TEXT NOT NULL,
  mode TEXT NOT NULL CHECK (mode IN ('chat', 'deep_research')),
  effort TEXT NOT NULL CHECK (effort IN ('low', 'medium', 'high')),
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, model_id, mode),
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  FOREIGN KEY (model_id) REFERENCES models(id) ON DELETE CASCADE
);
