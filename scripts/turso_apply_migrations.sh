#!/usr/bin/env bash
set -euo pipefail

if ! command -v turso >/dev/null 2>&1; then
  echo "error: turso CLI is not installed or not in PATH" >&2
  exit 1
fi

DB_NAME="${1:-${TURSO_DB_NAME:-}}"
if [[ -z "${DB_NAME}" ]]; then
  echo "usage: $0 <db-name>" >&2
  echo "or set TURSO_DB_NAME" >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
MIGRATIONS_DIR="${REPO_ROOT}/db/migrations"

echo "Applying migrations to Turso database '${DB_NAME}'"
for file in "${MIGRATIONS_DIR}"/*.sql; do
  echo "-> $(basename "${file}")"
  turso db shell "${DB_NAME}" < "${file}"
done

echo "Done."
