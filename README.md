# chat

Monorepo for the chat app.

## Quick start

1. Configure backend env:

```bash
cp backend/.env.example backend/.env
```

Populate `TURSO_DATABASE_URL` and `TURSO_AUTH_TOKEN` from the production `chat` database.
Set `GOOGLE_CLIENT_ID` during the final auth rollout phase.

3. Run backend + frontend in separate terminals:

```bash
./scripts/dev_backend.sh
./scripts/dev_frontend.sh
```

## API contract

OpenAPI spec: `backend/openapi/openapi.yaml`.
