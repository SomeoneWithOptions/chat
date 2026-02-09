# Infra

Deployment and infrastructure notes.

## Cloud Run backend

Deployment script:

```bash
ENV_FILE=/path/to/cloud-run.env.yaml ./scripts/deploy_cloud_run_backend.sh
```

Expected env vars in env file:

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

Current deployed service defaults:

- Project: `chat-486915`
- Region: `us-east1`
- Service: `chat-backend`
