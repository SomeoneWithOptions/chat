# Scripts

Utility scripts for local development and DB lifecycle.

## Local dev

- `./scripts/dev_backend.sh`: run Go API from `/backend` (loads `backend/.env` if present).
- `./scripts/dev_frontend.sh`: install frontend deps if needed and run Vite.

## Turso

- `./scripts/turso_create_db.sh <db-name>`: create a Turso DB if it does not already exist.
- `./scripts/turso_apply_migrations.sh <db-name>`: apply SQL files in `db/migrations` using Turso shell.
