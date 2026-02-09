# Infra

Deployment and infrastructure notes.

## Cloud Run backend

Deployment script:

```bash
./scripts/deploy_cloud_run_backend.sh
```

Optional deployment with explicit env file override:

```bash
./scripts/deploy_cloud_run_backend.sh --env-file /path/to/cloud-run.env.yaml
```

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
- `TURSO_DATABASE_URL`
- `TURSO_AUTH_TOKEN`
- `OPENROUTER_API_KEY`
- `BRAVE_API_KEY`
- `OPENROUTER_FREE_TIER_DEFAULT_MODEL`
- `LOCAL_UPLOAD_DIR`
- `DEEP_RESEARCH_TIMEOUT_SECONDS`

Auth rollout sequencing note:

- Keep auth-related values (`GOOGLE_CLIENT_ID`, `ALLOWED_GOOGLE_EMAILS`, `AUTH_REQUIRED`) as final rollout toggles after core feature stabilization.

Current deployed service defaults:

- Project: `chat-486915`
- Region: `us-east1`
- Service: `chat-backend`
