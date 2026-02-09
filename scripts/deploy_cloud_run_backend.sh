#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
SOURCE_DIR="${SOURCE_DIR:-${REPO_ROOT}/backend}"
SERVICE_NAME="${SERVICE_NAME:-chat-backend}"
PROJECT_ID="${PROJECT_ID:-}"
REGION="${REGION:-}"
ENV_FILE="${ENV_FILE:-}"
ALLOW_UNAUTHENTICATED="${ALLOW_UNAUTHENTICATED:-true}"
DRY_RUN=false

usage() {
  cat <<EOF
Usage:
  ./scripts/deploy_cloud_run_backend.sh [options]

Options:
  --env-file <path>       Optional Cloud Run env yaml file.
  --project <project-id>  GCP project id (default: ${PROJECT_ID}).
  --service <name>        Cloud Run service name (default: ${SERVICE_NAME}).
  --region <region>       Cloud Run region (default: ${REGION}).
  --source <dir>          Backend source directory (default: ${SOURCE_DIR}).
  --private               Do not pass --allow-unauthenticated.
  --dry-run               Print deploy command only.
  -h, --help              Show this help.

Environment variables:
  PROJECT_ID, SERVICE_NAME, REGION, SOURCE_DIR, ENV_FILE, ALLOW_UNAUTHENTICATED
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --env-file)
      ENV_FILE="${2:-}"
      shift 2
      ;;
    --project)
      PROJECT_ID="${2:-}"
      shift 2
      ;;
    --service)
      SERVICE_NAME="${2:-}"
      shift 2
      ;;
    --region)
      REGION="${2:-}"
      shift 2
      ;;
    --source)
      SOURCE_DIR="${2:-}"
      shift 2
      ;;
    --private)
      ALLOW_UNAUTHENTICATED="false"
      shift
      ;;
    --dry-run)
      DRY_RUN=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "error: unknown option: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if ! command -v gcloud >/dev/null 2>&1; then
  echo "error: gcloud CLI is required" >&2
  exit 1
fi

if [[ -z "${PROJECT_ID}" ]]; then
  PROJECT_ID="$(gcloud config get-value core/project 2>/dev/null || true)"
fi

if [[ -z "${REGION}" ]]; then
  REGION="$(gcloud config get-value run/region 2>/dev/null || true)"
fi

if [[ -z "${PROJECT_ID}" || "${PROJECT_ID}" == "(unset)" ]]; then
  PROJECT_ID="chat-486915"
fi

if [[ -z "${REGION}" || "${REGION}" == "(unset)" ]]; then
  REGION="us-east1"
fi

if ! gcloud auth list --filter=status:ACTIVE --format="value(account)" | grep -q .; then
  echo "error: no active gcloud account. run: gcloud auth login" >&2
  exit 1
fi

if [[ ! -d "${SOURCE_DIR}" ]]; then
  echo "error: source directory not found: ${SOURCE_DIR}" >&2
  exit 1
fi

if [[ -n "${ENV_FILE}" && ! -f "${ENV_FILE}" ]]; then
  echo "error: env file not found: ${ENV_FILE}" >&2
  exit 1
fi

deploy_cmd=(
  gcloud run deploy "${SERVICE_NAME}"
  --project "${PROJECT_ID}"
  --region "${REGION}"
  --source "${SOURCE_DIR}"
  --quiet
)

if [[ "${ALLOW_UNAUTHENTICATED}" == "true" ]]; then
  deploy_cmd+=(--allow-unauthenticated)
fi

if [[ -n "${ENV_FILE}" ]]; then
  deploy_cmd+=(--env-vars-file "${ENV_FILE}")
else
  echo "info: no env file provided; existing Cloud Run environment variables will be reused."
fi

git_sha="$(git -C "${REPO_ROOT}" rev-parse --short HEAD 2>/dev/null || true)"
if [[ -n "${git_sha}" ]]; then
  deploy_cmd+=(--update-labels "managed_by=script,component=backend,commit_sha=${git_sha}")
fi

if [[ "${DRY_RUN}" == "true" ]]; then
  printf "dry-run:"
  printf " %q" "${deploy_cmd[@]}"
  printf "\n"
  exit 0
fi

printf "Deploying service=%s project=%s region=%s\n" "${SERVICE_NAME}" "${PROJECT_ID}" "${REGION}"
"${deploy_cmd[@]}"

service_url="$(
  gcloud run services describe "${SERVICE_NAME}" \
    --project "${PROJECT_ID}" \
    --region "${REGION}" \
    --format="value(status.url)"
)"

printf "Deployed %s to %s\n" "${SERVICE_NAME}" "${service_url}"
