#!/usr/bin/env bash
set -euo pipefail

PROJECT_ID="${PROJECT_ID:-}"
PROJECT_NUMBER="${PROJECT_NUMBER:-}"
REPO="${REPO:-SomeoneWithOptions/chat}"
BRANCH_REF="${BRANCH_REF:-refs/heads/main}"
POOL_ID="${POOL_ID:-github-actions-pool}"
PROVIDER_ID="${PROVIDER_ID:-github-actions-provider}"
DEPLOYER_SA_NAME="${DEPLOYER_SA_NAME:-chat-backend-gha-deployer}"
REGION="${REGION:-us-east1}"
ARTIFACT_REPOSITORY="${ARTIFACT_REPOSITORY:-cloud-run-source-deploy}"
SERVICE_NAME="${SERVICE_NAME:-chat-backend}"
RUNTIME_SA="${RUNTIME_SA:-}"

usage() {
  cat <<EOF
Usage:
  ./scripts/setup_github_oidc_cloud_run_backend.sh [options]

Create/update GCP resources for GitHub Actions OIDC federation and backend Cloud Run deploys.

Options:
  --project <project-id>              GCP project id (default: gcloud config project or chat-486915).
  --project-number <number>           GCP project number (default: resolved from project id).
  --repo <owner/repo>                 GitHub repository allowed to deploy (default: ${REPO}).
  --branch <refs/heads/...>           Git branch ref allowed by provider (default: ${BRANCH_REF}).
  --pool-id <id>                      Workload Identity Pool id (default: ${POOL_ID}).
  --provider-id <id>                  Workload Identity Provider id (default: ${PROVIDER_ID}).
  --deployer-sa-name <name>           Service account name for deploys (default: ${DEPLOYER_SA_NAME}).
  --region <region>                   Artifact Registry/Cloud Run region (default: ${REGION}).
  --artifact-repo <repo>              Artifact Registry docker repo (default: ${ARTIFACT_REPOSITORY}).
  --service <service-name>            Cloud Run service name to inspect for runtime SA (default: ${SERVICE_NAME}).
  --runtime-sa <email>                Cloud Run runtime service account email (default: auto-detect from service).
  -h, --help                          Show help.

Environment variables:
  PROJECT_ID, PROJECT_NUMBER, REPO, BRANCH_REF, POOL_ID, PROVIDER_ID,
  DEPLOYER_SA_NAME, REGION, ARTIFACT_REPOSITORY, SERVICE_NAME, RUNTIME_SA
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --project)
      PROJECT_ID="${2:-}"
      shift 2
      ;;
    --project-number)
      PROJECT_NUMBER="${2:-}"
      shift 2
      ;;
    --repo)
      REPO="${2:-}"
      shift 2
      ;;
    --branch)
      BRANCH_REF="${2:-}"
      shift 2
      ;;
    --pool-id)
      POOL_ID="${2:-}"
      shift 2
      ;;
    --provider-id)
      PROVIDER_ID="${2:-}"
      shift 2
      ;;
    --deployer-sa-name)
      DEPLOYER_SA_NAME="${2:-}"
      shift 2
      ;;
    --region)
      REGION="${2:-}"
      shift 2
      ;;
    --artifact-repo)
      ARTIFACT_REPOSITORY="${2:-}"
      shift 2
      ;;
    --service)
      SERVICE_NAME="${2:-}"
      shift 2
      ;;
    --runtime-sa)
      RUNTIME_SA="${2:-}"
      shift 2
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
if [[ -z "${PROJECT_ID}" || "${PROJECT_ID}" == "(unset)" ]]; then
  PROJECT_ID="chat-486915"
fi

if [[ -z "${PROJECT_NUMBER}" ]]; then
  PROJECT_NUMBER="$(gcloud projects describe "${PROJECT_ID}" --format="value(projectNumber)")"
fi

if [[ -z "${RUNTIME_SA}" ]]; then
  RUNTIME_SA="$(
    gcloud run services describe "${SERVICE_NAME}" \
      --project "${PROJECT_ID}" \
      --region "${REGION}" \
      --format="value(spec.template.spec.serviceAccountName)" 2>/dev/null || true
  )"
fi

if [[ -z "${RUNTIME_SA}" ]]; then
  echo "error: runtime service account could not be auto-detected." >&2
  echo "       pass --runtime-sa explicitly." >&2
  exit 1
fi

if ! gcloud auth list --filter=status:ACTIVE --format="value(account)" | grep -q .; then
  echo "error: no active gcloud account. run: gcloud auth login" >&2
  exit 1
fi

DEPLOYER_SA_EMAIL="${DEPLOYER_SA_NAME}@${PROJECT_ID}.iam.gserviceaccount.com"

echo "Enabling required APIs in project ${PROJECT_ID}"
gcloud services enable \
  iam.googleapis.com \
  iamcredentials.googleapis.com \
  sts.googleapis.com \
  run.googleapis.com \
  artifactregistry.googleapis.com \
  --project "${PROJECT_ID}" >/dev/null

echo "Ensuring Workload Identity Pool ${POOL_ID} exists"
if ! gcloud iam workload-identity-pools describe "${POOL_ID}" --project "${PROJECT_ID}" --location global >/dev/null 2>&1; then
  gcloud iam workload-identity-pools create "${POOL_ID}" \
    --project "${PROJECT_ID}" \
    --location global \
    --display-name "GitHub Actions Pool" >/dev/null
fi

echo "Ensuring Workload Identity Provider ${PROVIDER_ID} exists with repo/branch restriction"
if ! gcloud iam workload-identity-pools providers describe "${PROVIDER_ID}" \
  --project "${PROJECT_ID}" \
  --location global \
  --workload-identity-pool "${POOL_ID}" >/dev/null 2>&1; then
  gcloud iam workload-identity-pools providers create-oidc "${PROVIDER_ID}" \
    --project "${PROJECT_ID}" \
    --location global \
    --workload-identity-pool "${POOL_ID}" \
    --display-name "GitHub OIDC Provider" \
    --issuer-uri "https://token.actions.githubusercontent.com" \
    --attribute-mapping "google.subject=assertion.sub,attribute.actor=assertion.actor,attribute.repository=assertion.repository,attribute.repository_owner=assertion.repository_owner,attribute.ref=assertion.ref,attribute.workflow_ref=assertion.job_workflow_ref" \
    --attribute-condition "assertion.repository=='${REPO}' && assertion.ref=='${BRANCH_REF}'" >/dev/null
else
  gcloud iam workload-identity-pools providers update-oidc "${PROVIDER_ID}" \
    --project "${PROJECT_ID}" \
    --location global \
    --workload-identity-pool "${POOL_ID}" \
    --attribute-mapping "google.subject=assertion.sub,attribute.actor=assertion.actor,attribute.repository=assertion.repository,attribute.repository_owner=assertion.repository_owner,attribute.ref=assertion.ref,attribute.workflow_ref=assertion.job_workflow_ref" \
    --attribute-condition "assertion.repository=='${REPO}' && assertion.ref=='${BRANCH_REF}'" >/dev/null
fi

echo "Ensuring deployer service account ${DEPLOYER_SA_EMAIL} exists"
if ! gcloud iam service-accounts describe "${DEPLOYER_SA_EMAIL}" --project "${PROJECT_ID}" >/dev/null 2>&1; then
  gcloud iam service-accounts create "${DEPLOYER_SA_NAME}" \
    --project "${PROJECT_ID}" \
    --display-name "Chat Backend GitHub Deployer" >/dev/null
fi

echo "Granting deploy permissions to ${DEPLOYER_SA_EMAIL}"
gcloud projects add-iam-policy-binding "${PROJECT_ID}" \
  --member "serviceAccount:${DEPLOYER_SA_EMAIL}" \
  --role "roles/run.admin" \
  --condition=None >/dev/null

gcloud artifacts repositories add-iam-policy-binding "${ARTIFACT_REPOSITORY}" \
  --project "${PROJECT_ID}" \
  --location "${REGION}" \
  --member "serviceAccount:${DEPLOYER_SA_EMAIL}" \
  --role "roles/artifactregistry.writer" >/dev/null

gcloud iam service-accounts add-iam-policy-binding "${RUNTIME_SA}" \
  --project "${PROJECT_ID}" \
  --member "serviceAccount:${DEPLOYER_SA_EMAIL}" \
  --role "roles/iam.serviceAccountUser" >/dev/null

echo "Allowing GitHub repo principal to impersonate ${DEPLOYER_SA_EMAIL}"
gcloud iam service-accounts add-iam-policy-binding "${DEPLOYER_SA_EMAIL}" \
  --project "${PROJECT_ID}" \
  --member "principalSet://iam.googleapis.com/projects/${PROJECT_NUMBER}/locations/global/workloadIdentityPools/${POOL_ID}/attribute.repository/${REPO}" \
  --role "roles/iam.workloadIdentityUser" >/dev/null

echo
echo "Setup complete. Configure these GitHub repository variables:"
echo "GCP_WORKLOAD_IDENTITY_PROVIDER=projects/${PROJECT_NUMBER}/locations/global/workloadIdentityPools/${POOL_ID}/providers/${PROVIDER_ID}"
echo "GCP_DEPLOY_SERVICE_ACCOUNT=${DEPLOYER_SA_EMAIL}"
echo "GCP_PROJECT_ID=${PROJECT_ID}"
echo "GCP_REGION=${REGION}"
echo "CLOUD_RUN_SERVICE=${SERVICE_NAME}"
echo "ARTIFACT_REGISTRY_REPOSITORY=${ARTIFACT_REPOSITORY}"
echo "BACKEND_IMAGE_NAME=chat-backend"
