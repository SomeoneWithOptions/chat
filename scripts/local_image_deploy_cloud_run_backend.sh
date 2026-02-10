#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
SERVICE_NAME="${SERVICE_NAME:-chat-backend}"
PROJECT_ID="${PROJECT_ID:-}"
REGION="${REGION:-}"
DOCKER_CONTEXT="${DOCKER_CONTEXT:-${REPO_ROOT}/backend}"
DOCKERFILE="${DOCKERFILE:-${DOCKER_CONTEXT}/Dockerfile}"
ENV_FILE="${ENV_FILE:-}"
IMAGE_REPO="${IMAGE_REPO:-}"
IMAGE_TAG="${IMAGE_TAG:-}"
ALLOW_UNAUTHENTICATED="${ALLOW_UNAUTHENTICATED:-true}"
PLATFORM="${PLATFORM:-linux/amd64}"
DRY_RUN=false
SCRIPT_START_EPOCH="$(date +%s)"

usage() {
  cat <<EOF
Usage:
  ./scripts/deploy_cloud_run_backend_image.sh [options]

Build backend image locally, push to Artifact Registry, then deploy existing Cloud Run service.

Options:
  --env-file <path>       Optional Cloud Run env yaml file.
  --project <project-id>  GCP project id (default: ${PROJECT_ID}).
  --service <name>        Cloud Run service name (default: ${SERVICE_NAME}).
  --region <region>       Cloud Run region (default: ${REGION}).
  --image-repo <repo>     Full image repo without tag (example: us-east1-docker.pkg.dev/p/r/chat-backend).
  --tag <tag>             Image tag to publish (default: utc timestamp + git sha).
  --context <dir>         Docker build context (default: ${DOCKER_CONTEXT}).
  --dockerfile <path>     Dockerfile path (default: ${DOCKERFILE}).
  --private               Do not pass --allow-unauthenticated.
  --dry-run               Print commands only.
  -h, --help              Show this help.

Environment variables:
  PROJECT_ID, SERVICE_NAME, REGION, DOCKER_CONTEXT, DOCKERFILE, ENV_FILE,
  IMAGE_REPO, IMAGE_TAG, PLATFORM, ALLOW_UNAUTHENTICATED
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
    --image-repo)
      IMAGE_REPO="${2:-}"
      shift 2
      ;;
    --tag)
      IMAGE_TAG="${2:-}"
      shift 2
      ;;
    --context)
      DOCKER_CONTEXT="${2:-}"
      shift 2
      ;;
    --dockerfile)
      DOCKERFILE="${2:-}"
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

if ! command -v docker >/dev/null 2>&1; then
  echo "error: docker CLI is required" >&2
  exit 1
fi

if ! docker buildx version >/dev/null 2>&1; then
  echo "error: docker buildx is required" >&2
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

if [[ ! -d "${DOCKER_CONTEXT}" ]]; then
  echo "error: docker context directory not found: ${DOCKER_CONTEXT}" >&2
  exit 1
fi

if [[ ! -f "${DOCKERFILE}" ]]; then
  echo "error: dockerfile not found: ${DOCKERFILE}" >&2
  exit 1
fi

if [[ -n "${ENV_FILE}" && ! -f "${ENV_FILE}" ]]; then
  echo "error: env file not found: ${ENV_FILE}" >&2
  exit 1
fi

if [[ "${PLATFORM}" != "linux/amd64" ]]; then
  echo "error: this script enforces PLATFORM=linux/amd64 for Cloud Run compatibility." >&2
  echo "       current Dockerfile builds an amd64 binary." >&2
  exit 1
fi

if [[ -z "${IMAGE_REPO}" ]]; then
  current_image="$(
    gcloud run services describe "${SERVICE_NAME}" \
      --project "${PROJECT_ID}" \
      --region "${REGION}" \
      --format="value(spec.template.spec.containers[0].image)" 2>/dev/null || true
  )"
  if [[ -z "${current_image}" ]]; then
    echo "error: unable to determine current service image for ${SERVICE_NAME}." >&2
    echo "       pass --image-repo explicitly (for example: us-east1-docker.pkg.dev/p/r/chat-backend)." >&2
    exit 1
  fi
  without_digest="${current_image%@*}"
  IMAGE_REPO="${without_digest%:*}"
fi

if [[ -z "${IMAGE_TAG}" ]]; then
  timestamp="$(date -u +%Y%m%d-%H%M%S)"
  git_sha="$(git -C "${REPO_ROOT}" rev-parse --short HEAD 2>/dev/null || echo "nogit")"
  IMAGE_TAG="${timestamp}-${git_sha}"
fi

IMAGE_URI="${IMAGE_REPO}:${IMAGE_TAG}"
REGISTRY_HOST="${IMAGE_REPO%%/*}"

build_cmd=(
  docker buildx build
  --platform "${PLATFORM}"
  --file "${DOCKERFILE}"
  --tag "${IMAGE_URI}"
  --load
  "${DOCKER_CONTEXT}"
)

push_cmd=(docker push "${IMAGE_URI}")

deploy_cmd=(
  gcloud run deploy "${SERVICE_NAME}"
  --project "${PROJECT_ID}"
  --region "${REGION}"
  --image "${IMAGE_URI}"
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

if [[ "${DRY_RUN}" == "true" ]]; then
  echo "dry-run: gcloud auth configure-docker ${REGISTRY_HOST} --quiet"
  printf "dry-run:"
  printf " %q" "${build_cmd[@]}"
  printf "\n"
  printf "dry-run:"
  printf " %q" "${push_cmd[@]}"
  printf "\n"
  printf "dry-run:"
  printf " %q" "${deploy_cmd[@]}"
  printf "\n"
  exit 0
fi

echo "Configuring Docker auth for ${REGISTRY_HOST}"
gcloud auth configure-docker "${REGISTRY_HOST}" --quiet >/dev/null

printf "Building image=%s platform=%s\n" "${IMAGE_URI}" "${PLATFORM}"
"${build_cmd[@]}"

printf "Pushing image=%s\n" "${IMAGE_URI}"
"${push_cmd[@]}"

printf "Deploying service=%s project=%s region=%s image=%s\n" "${SERVICE_NAME}" "${PROJECT_ID}" "${REGION}" "${IMAGE_URI}"
"${deploy_cmd[@]}"

service_url="$(
  gcloud run services describe "${SERVICE_NAME}" \
    --project "${PROJECT_ID}" \
    --region "${REGION}" \
    --format="value(status.url)"
)"

deployed_image="$(
  gcloud run services describe "${SERVICE_NAME}" \
    --project "${PROJECT_ID}" \
    --region "${REGION}" \
    --format="value(spec.template.spec.containers[0].image)"
)"

printf "Deployed %s to %s\n" "${SERVICE_NAME}" "${service_url}"
printf "Active image: %s\n" "${deployed_image}"
printf "Cloud Run compatibility: built with --platform=%s\n" "${PLATFORM}"

elapsed_seconds="$(( $(date +%s) - SCRIPT_START_EPOCH ))"
elapsed_hours="$(( elapsed_seconds / 3600 ))"
elapsed_minutes="$(( (elapsed_seconds % 3600) / 60 ))"
elapsed_remainder_seconds="$(( elapsed_seconds % 60 ))"
printf "Total runtime: %02d:%02d:%02d (%ss)\n" \
  "${elapsed_hours}" "${elapsed_minutes}" "${elapsed_remainder_seconds}" "${elapsed_seconds}"
