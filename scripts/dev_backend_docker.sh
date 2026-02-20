#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

cd "${REPO_ROOT}/backend"

echo "Building Docker image 'chat-backend:local'..."
docker build -t chat-backend:local .

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

# Configure Auth for Local Dev (Bypass Google Verification)
export AUTH_REQUIRED=true
export AUTH_INSECURE_SKIP_GOOGLE_VERIFY=true

echo "Starting Backend in Docker..."
echo "  Database: ${TURSO_DATABASE_URL}"
echo "  Auth Mode: Local Dev (Insecure Bypass)"

DOCKER_CMD=(docker run --rm -p 8080:8080)
DOCKER_CMD+=(-e "TURSO_DATABASE_URL=${TURSO_DATABASE_URL}")
DOCKER_CMD+=(-e "TURSO_AUTH_TOKEN=${TURSO_AUTH_TOKEN}")
DOCKER_CMD+=(-e "AUTH_REQUIRED=${AUTH_REQUIRED}")
DOCKER_CMD+=(-e "AUTH_INSECURE_SKIP_GOOGLE_VERIFY=${AUTH_INSECURE_SKIP_GOOGLE_VERIFY}")

if [[ -f .env ]]; then
  echo "  Using .env file"
  DOCKER_CMD+=(--env-file .env)
fi

DOCKER_CMD+=(chat-backend:local)

"${DOCKER_CMD[@]}"
