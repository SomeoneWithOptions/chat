-- 0002_seed_default_model.sql
-- Seed fallback model required by product invariant.

INSERT INTO models (
  id,
  provider,
  display_name,
  context_window,
  prompt_price_microusd,
  completion_price_microusd,
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
  1,
  1
)
ON CONFLICT(id) DO UPDATE SET
  provider = excluded.provider,
  display_name = excluded.display_name,
  curated = excluded.curated,
  is_active = excluded.is_active,
  updated_at = CURRENT_TIMESTAMP;
