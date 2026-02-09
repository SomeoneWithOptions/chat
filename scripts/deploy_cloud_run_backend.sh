#!/usr/bin/env bash
set -euo pipefail

PROJECT_ID="${PROJECT_ID:-chat-486915}"
SERVICE_NAME="${SERVICE_NAME:-chat-backend}"
REGION="${REGION:-us-east1}"

if ! command -v gcloud >/dev/null 2>&1; then
  echo "error: gcloud CLI is required" >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
ENV_FILE="${ENV_FILE:-}"

if [[ -z "${ENV_FILE}" ]]; then
  echo "usage: ENV_FILE=/path/to/cloud-run.env ./scripts/deploy_cloud_run_backend.sh" >&2
  exit 1
fi

if [[ ! -f "${ENV_FILE}" ]]; then
  echo "error: env file not found: ${ENV_FILE}" >&2
  exit 1
fi

gcloud run deploy "${SERVICE_NAME}" \
  --project "${PROJECT_ID}" \
  --region "${REGION}" \
  --source "${REPO_ROOT}/backend" \
  --allow-unauthenticated \
  --env-vars-file "${ENV_FILE}"
