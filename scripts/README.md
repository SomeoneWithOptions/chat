# Scripts

Utility scripts for local development, DB lifecycle, and deployment.

## Local dev

- `./scripts/dev_backend.sh`: run Go API from `/backend` (loads `backend/.env` if present).
- `./scripts/dev_frontend.sh`: install frontend deps if needed and run Vite.

## Turso

- `./scripts/turso_create_db.sh <db-name>`: create a Turso DB if it does not already exist.
- `./scripts/turso_apply_migrations.sh <db-name>`: apply SQL files in `db/migrations` using Turso shell.

## Cloud Run

- `./scripts/deploy_cloud_run_backend.sh`: deploy backend source to Cloud Run using existing Cloud Run env vars.
- `./scripts/deploy_cloud_run_backend.sh --env-file /path/to/env.yaml`: deploy and replace env vars from a file.
- Common overrides: `--project`, `--region`, `--service`, `--source`, `--private`, `--dry-run`.
- `./scripts/local_image_deploy_cloud_run_backend.sh`: build backend image locally (`linux/amd64`), push to Artifact Registry, and update Cloud Run service image.
- `./scripts/local_image_deploy_cloud_run_backend.sh --env-file /path/to/env.yaml`: image-based deploy with env file replacement.
- Common overrides: `--project`, `--region`, `--service`, `--image-repo`, `--tag`, `--context`, `--dockerfile`, `--private`, `--dry-run`.
- Cloud Run compatibility guard: image builds are enforced as `linux/amd64`.
- `./scripts/setup_github_oidc_cloud_run_backend.sh`: one-time setup for GitHub Actions Workload Identity Federation, deploy IAM bindings, and Artifact Registry image cleanup policy.
- Cleanup policy default keeps the latest 10 images in the Artifact Registry repo; override with `--artifact-keep-latest <count>`.
