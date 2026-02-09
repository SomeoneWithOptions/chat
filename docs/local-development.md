# Local Development

## Prerequisites

- Go 1.22+
- Bun 1.0+
- Turso CLI (`turso`) authenticated with an account that can create DBs

## 1) Create and migrate Turso database

```bash
./scripts/turso_create_db.sh chat-dev
./scripts/turso_apply_migrations.sh chat-dev
```

Get connection values:

```bash
turso db show chat-dev --url
turso db tokens create chat-dev
```

## 2) Configure backend

```bash
cp backend/.env.example backend/.env
```

Set at minimum:

- `TURSO_DATABASE_URL`
- `TURSO_AUTH_TOKEN` (required for `libsql://` URL)
- `GOOGLE_CLIENT_ID` (required only when `AUTH_REQUIRED=true`)

For temporary auth bypass while testing app flows:

- `AUTH_REQUIRED=false`

For local dev-only login flow:

- `AUTH_INSECURE_SKIP_GOOGLE_VERIFY=true`

## 3) Run backend and frontend

```bash
./scripts/dev_backend.sh
./scripts/dev_frontend.sh
```

- Backend default: `http://localhost:8080`
- Frontend default: `http://localhost:5173`

## 4) Validate health

```bash
curl http://localhost:8080/healthz
```

Expected response:

```json
{"status":"ok"}
```
