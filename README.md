# chat

Monorepo for the Saneto Chat app.

## Quick start

1. Create a Turso DB:

```bash
./scripts/turso_create_db.sh chat-dev
./scripts/turso_apply_migrations.sh chat-dev
```

2. Configure backend env:

```bash
cp backend/.env.example backend/.env
```

Populate `TURSO_DATABASE_URL`, `TURSO_AUTH_TOKEN`, and `GOOGLE_CLIENT_ID`.

3. Run backend + frontend in separate terminals:

```bash
./scripts/dev_backend.sh
./scripts/dev_frontend.sh
```

## API contract

OpenAPI spec: `backend/openapi/openapi.yaml`.
