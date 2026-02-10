-- 0002_seed_default_model.sql
-- Seed fallback model required by product invariant.

INSERT INTO models (
  id,
  provider,
  display_name,
  context_window,
  prompt_price_microusd,
  completion_price_microusd,
  supported_parameters_json,
  supports_reasoning,
  curated,
  is_active
)
VALUES (
  'openrouter/free',
  'openrouter',
  'OpenRouter Free',
  0,
  0,
  0,
  '["reasoning"]',
  1,
  1,
  1
)
ON CONFLICT(id) DO UPDATE SET
  provider = excluded.provider,
  display_name = excluded.display_name,
  supported_parameters_json = excluded.supported_parameters_json,
  supports_reasoning = excluded.supports_reasoning,
  curated = excluded.curated,
  is_active = excluded.is_active,
  updated_at = CURRENT_TIMESTAMP;
