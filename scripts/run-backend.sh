#!/usr/bin/env bash
set -euo pipefail
task_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
source "$task_root/scripts/load-env.sh"
cd "$task_root/backend"
case "${1:-serve}" in
  serve) exec go run ./cmd/server ;;
  migrate) exec go run ./cmd/server migrate ;;
  seed-admin) exec go run ./cmd/seed-admin ;;
  *) printf '%s\n' 'Usage: bash scripts/run-backend.sh [serve|migrate|seed-admin]' >&2; exit 1 ;;
esac
