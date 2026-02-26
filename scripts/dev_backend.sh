#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

cd "${REPO_ROOT}/backend"

if [[ -f .env ]]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi

# Fetch Turso credentials if not provided
if [ -z "${TURSO_DATABASE_URL:-}" ]; then
  echo "Fetching Turso Database URL for 'chat-prod'..."
  # Check if turso CLI is available
  if ! command -v turso &> /dev/null; then
    echo "Error: 'turso' CLI not found. Please install it or set TURSO_DATABASE_URL manually."
    exit 1
  fi
  TURSO_DATABASE_URL=$(turso db show chat-prod --url)
  export TURSO_DATABASE_URL
fi

if [ -z "${TURSO_AUTH_TOKEN:-}" ]; then
  echo "Fetching Turso Auth Token for 'chat-prod'..."
  TURSO_AUTH_TOKEN=$(turso db tokens create chat-prod)
  export TURSO_AUTH_TOKEN
fi

# Configure auth for local dev (bypass Google token verification)
export AUTH_REQUIRED=true
export AUTH_INSECURE_SKIP_GOOGLE_VERIFY=true

echo "Starting Backend..."
echo "  Database: ${TURSO_DATABASE_URL}"
echo "  Auth Mode: Local Dev (Insecure Bypass)"

go run ./cmd/api
