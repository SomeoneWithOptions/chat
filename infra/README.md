# Infra

Deployment and infrastructure notes.

## Cloud Run backend

Deployment script:

```bash
./scripts/deploy_cloud_run_backend.sh
```

Image-based deploy script (local build -> Artifact Registry -> Cloud Run):

```bash
./scripts/local_image_deploy_cloud_run_backend.sh
```

The image deploy script automatically reuses the currently deployed service image repository when `--image-repo` is not provided.

Optional deployment with explicit env file override:

```bash
./scripts/deploy_cloud_run_backend.sh --env-file /path/to/cloud-run.env.yaml
./scripts/local_image_deploy_cloud_run_backend.sh --env-file /path/to/cloud-run.env.yaml
```

## GitHub Actions deploy (OIDC federation)

Workflow:

- `.github/workflows/deploy_backend_cloud_run.yml`

One-time GCP setup helper:

```bash
./scripts/setup_github_oidc_cloud_run_backend.sh
```

The setup helper also applies an Artifact Registry cleanup policy on the configured repo, keeping the latest 10 images by default (override with `--artifact-keep-latest`).

Recommended GitHub repository variables:

- `GCP_WORKLOAD_IDENTITY_PROVIDER` (example: `projects/1011074047731/locations/global/workloadIdentityPools/github-actions-pool/providers/github-actions-provider`)
- `GCP_DEPLOY_SERVICE_ACCOUNT` (example: `chat-backend-gha-deployer@chat-486915.iam.gserviceaccount.com`)
- `GCP_PROJECT_ID` (default in workflow: `chat-486915`)
- `GCP_REGION` (default in workflow: `us-east1`)
- `CLOUD_RUN_SERVICE` (default in workflow: `chat-backend`)
- `ARTIFACT_REGISTRY_REPOSITORY` (default in workflow: `cloud-run-source-deploy`)
- `BACKEND_IMAGE_NAME` (default in workflow: `chat-backend`)
- `CLOUD_RUN_ALLOW_UNAUTHENTICATED` (`true`/`false`, default: `true`)

Starter template:

- `infra/cloud-run.env.example.yaml`

Expected env vars in the env file:

- `APP_ENV`
- `FRONTEND_ORIGIN`
- `CORS_ALLOWED_ORIGINS`
- `COOKIE_SECURE`
- `SESSION_COOKIE_NAME`
- `SESSION_TTL_HOURS`
- `ALLOWED_GOOGLE_EMAILS`
- `GOOGLE_CLIENT_ID`
- `AUTH_REQUIRED`
- `AUTH_INSECURE_SKIP_GOOGLE_VERIFY`
- `MODEL_SYNC_BEARER_TOKEN`
- `TURSO_DATABASE_URL`
- `TURSO_AUTH_TOKEN`
- `OPENROUTER_API_KEY`
- `BRAVE_API_KEY`
- `OPENROUTER_FREE_TIER_DEFAULT_MODEL` (first-run/default fallback model; request-selected model is used when provided)
- `LOCAL_UPLOAD_DIR`
- `DEEP_RESEARCH_TIMEOUT_SECONDS`

Auth rollout sequencing note:

- Keep auth-related values (`GOOGLE_CLIENT_ID`, `ALLOWED_GOOGLE_EMAILS`, `AUTH_REQUIRED`) as final rollout toggles after core feature stabilization.

Current deployed service defaults:

- Project: `chat-486915`
- Region: `us-east1`
- Service: `chat-backend`
