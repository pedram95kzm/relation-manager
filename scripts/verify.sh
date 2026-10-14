#!/usr/bin/env bash
set -euo pipefail
task_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
if [[ "${1:-}" == '--require-database' && -z "${TEST_DATABASE_URL:-}" ]]; then
  printf '%s\n' 'Set TEST_DATABASE_URL to a dedicated disposable database before integration verification.' >&2
  exit 1
fi
if [[ -z "${TEST_DATABASE_URL:-}" ]]; then
  printf '%s\n' 'Warning: TEST_DATABASE_URL absent; PostgreSQL integration tests will be skipped.' >&2
fi
cd "$task_root/backend"
go mod verify
test -z "$(gofmt -l cmd internal migrations)"
go vet ./...
go test ./... -count=1
go build ./...
cd "$task_root/frontend"
npm ci
npm test
npm run build
printf '%s\n' 'Verification passed. Live providers, Google OAuth and Docker need their own runtime checks.'
