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

if turso db show "${DB_NAME}" >/dev/null 2>&1; then
  echo "database '${DB_NAME}' already exists"
else
  echo "creating database '${DB_NAME}'"
  turso db create "${DB_NAME}"
fi

echo
echo "Database details:"
turso db show "${DB_NAME}"

echo
echo "Create an auth token for backend usage with:"
echo "  turso db tokens create ${DB_NAME}"
echo
echo "Get the libsql URL with:"
echo "  turso db show ${DB_NAME} --url"
