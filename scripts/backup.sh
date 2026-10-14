#!/usr/bin/env bash
set -euo pipefail
task_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
task_mode=${1:-native}
task_directory=${2:-"$task_root/.local/backups"}
umask 077
mkdir -p -- "$task_directory"
task_directory=$(cd -- "$task_directory" && pwd)
task_dump_name="relations-$(date -u +%Y%m%d-%H%M%S)-$(openssl rand -hex 4).dump"
task_dump="$task_directory/$task_dump_name"
cd "$task_root"
case "$task_mode" in
  compose)
    task_container_dump="/tmp/$task_dump_name"
    trap 'docker compose exec -T database rm -f -- "$task_container_dump" >/dev/null' EXIT
    docker compose exec -T database pg_dump -U relations -d relations --format=custom --file="$task_container_dump"
    docker compose cp "database:$task_container_dump" "$task_dump"
    ;;
  native)
    source "$task_root/scripts/load-env.sh" "${3:-$task_root/.env}"
    task_python=python3
    if ! "$task_python" -c 'import sys; assert sys.version_info.major == 3' >/dev/null 2>&1; then
      task_python=python
      "$task_python" -c 'import sys; assert sys.version_info.major == 3' >/dev/null 2>&1 || { printf '%s\n' 'Python 3 is required to parse the database URL securely.' >&2; exit 1; }
    fi
    # The URL is inherited through the environment rather than placed in process arguments.
    "$task_python" - "$task_dump" <<'PY'
import os
import subprocess
import sys
from urllib.parse import parse_qs, unquote, urlsplit

url = urlsplit(os.environ.get("DATABASE_URL", ""))
if url.scheme not in ("postgres", "postgresql") or not url.hostname:
    sys.exit("DATABASE_URL must be a PostgreSQL URL.")
env = os.environ.copy()
env.update(PGHOST=url.hostname, PGPORT=str(url.port or 5432), PGDATABASE=unquote(url.path.lstrip("/")),
           PGUSER=unquote(url.username or ""), PGPASSWORD=unquote(url.password or ""), PGCONNECT_TIMEOUT="10", PGSSLMODE="prefer")
names = {"sslmode": "PGSSLMODE", "sslrootcert": "PGSSLROOTCERT", "sslcert": "PGSSLCERT", "sslkey": "PGSSLKEY",
         "connect_timeout": "PGCONNECT_TIMEOUT", "application_name": "PGAPPNAME"}
for key, values in parse_qs(url.query).items():
    if key in names:
        env[names[key]] = values[-1]
subprocess.run(["pg_dump", "--format=custom", "--file=" + sys.argv[1]], env=env, check=True)
subprocess.run(["pg_restore", "--list", sys.argv[1]], env=env, check=True, stdout=subprocess.DEVNULL)
PY
    ;;
  *) printf '%s\n' 'Usage: bash scripts/backup.sh [native|compose] [output-directory] [env-file]' >&2; exit 1 ;;
esac
printf 'Backup created: %s\n' "$task_dump"
printf '%s\n' 'Protect this unencrypted dump and test restoration into a separate empty database.'
