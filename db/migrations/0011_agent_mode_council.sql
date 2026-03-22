-- Up
ALTER TABLE user_model_preferences ADD COLUMN last_used_agent_source_model_ids_json TEXT;
ALTER TABLE user_model_preferences ADD COLUMN last_used_agent_fusion_model_id TEXT;

ALTER TABLE messages ADD COLUMN agent_sources_json TEXT;
ALTER TABLE messages ADD COLUMN agent_analysis_json TEXT;
ALTER TABLE messages ADD COLUMN agent_result_model_id TEXT;
ALTER TABLE messages ADD COLUMN agent_result_usage_json TEXT;
ALTER TABLE messages ADD COLUMN agent_run_id TEXT;

ALTER TABLE agent_runs ADD COLUMN workflow_type TEXT NOT NULL DEFAULT 'single_model';
ALTER TABLE agent_runs ADD COLUMN source_model_ids_json TEXT;
ALTER TABLE agent_runs ADD COLUMN fusion_model_id TEXT;
ALTER TABLE agent_runs ADD COLUMN grounding_enabled INTEGER NOT NULL DEFAULT 1;
ALTER TABLE agent_runs ADD COLUMN council_config_json TEXT;
ALTER TABLE agent_runs ADD COLUMN source_results_json TEXT;
ALTER TABLE agent_runs ADD COLUMN fusion_analysis_json TEXT;
ALTER TABLE agent_runs ADD COLUMN fusion_result_json TEXT;
ALTER TABLE agent_runs ADD COLUMN completed_sources INTEGER NOT NULL DEFAULT 0;
ALTER TABLE agent_runs ADD COLUMN degraded_sources INTEGER NOT NULL DEFAULT 0;
ALTER TABLE agent_runs ADD COLUMN failed_sources INTEGER NOT NULL DEFAULT 0;
ALTER TABLE agent_runs ADD COLUMN public_status_json TEXT;

-- Down
-- SQLite does not support dropping columns easily.
