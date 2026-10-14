#!/usr/bin/env bash
set -euo pipefail
task_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
task_output=${1:-"$task_root/.env"}
if [[ -e "$task_output" ]]; then
  printf '%s\n' 'Configuration already exists; existing settings and secrets were preserved.'
  exit 0
fi
command -v openssl >/dev/null || { printf '%s\n' 'OpenSSL is required to generate secrets.' >&2; exit 1; }
umask 077
task_jwt=$(openssl rand -hex 32)
task_key=$(openssl rand -base64 32)
task_password=$(openssl rand -hex 32)
# Refuse an output that appeared after the initial existence check.
set -o noclobber
{
  while IFS= read -r task_line || [[ -n "$task_line" ]]; do
    task_line=${task_line%$'\r'}
    case "$task_line" in
      JWT_SECRET=*) printf 'JWT_SECRET=%s\n' "$task_jwt" ;;
      AI_ENCRYPTION_KEY=*) printf 'AI_ENCRYPTION_KEY=%s\n' "$task_key" ;;
      POSTGRES_PASSWORD=*) printf 'POSTGRES_PASSWORD=%s\n' "$task_password" ;;
      DATABASE_URL=*) printf 'DATABASE_URL=postgres://relations:%s@127.0.0.1:5432/relations?sslmode=disable\n' "$task_password" ;;
      *) printf '%s\n' "$task_line" ;;
    esac
  done < "$task_root/.env.example"
} > "$task_output"
unset task_jwt task_key task_password task_line
printf '%s\n' 'Created private configuration with random secrets. Review the database URL, public URL and optional Google/admin settings before starting.'
