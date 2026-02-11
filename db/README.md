# DB

Database schema and migration scripts for Turso (LibSQL).

## Files

- `schema.sql`: canonical schema for MVP entities.
- `migrations/0001_init.sql`: base schema migration.
- `migrations/0002_seed_default_model.sql`: seeds `openrouter/free` fallback model.
- `migrations/0003_message_files_file_id_index.sql`: adds index to speed attachment cleanup by `file_id`.
- `migrations/0004_model_reasoning_presets.sql`: adds model reasoning capability fields and per-user model reasoning presets.

## Turso CLI usage

We use a single production database (`chat`). Migrations are applied to production.

Get backend env values:

- `TURSO_DATABASE_URL` from `turso db show chat --url`
- `TURSO_AUTH_TOKEN` from `turso db tokens create chat`
