# Scripts

Utility scripts for local development, DB lifecycle, and deployment.

## Local dev

- `./scripts/dev_backend.sh`: run Go API from `/backend` (loads `backend/.env` if present).
- `./scripts/dev_frontend.sh`: install frontend deps if needed and run Vite.

## Turso

- `./scripts/turso_create_db.sh <db-name>`: create a Turso DB if it does not already exist.
- `./scripts/turso_apply_migrations.sh <db-name>`: apply SQL files in `db/migrations` using Turso shell.

## Cloud Run

- `ENV_FILE=/path/to/env.yaml ./scripts/deploy_cloud_run_backend.sh`: deploy backend source to Cloud Run.
