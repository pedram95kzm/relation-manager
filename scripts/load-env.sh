#!/usr/bin/env bash
# Source this file to load configuration without evaluating shell commands.
task_env_path=${1:-"$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)/.env"}
if [[ ! -f "$task_env_path" ]]; then
  printf '%s\n' 'Configuration missing. Run bash scripts/setup-env.sh first.' >&2
  return 1 2>/dev/null || exit 1
fi
while IFS= read -r task_env_line || [[ -n "$task_env_line" ]]; do
  task_env_line=${task_env_line%$'\r'}
  [[ "$task_env_line" =~ ^[[:space:]]*(#|$) ]] && continue
  if [[ ! "$task_env_line" =~ ^([A-Z][A-Z0-9_]*)=(.*)$ ]]; then
    printf '%s\n' 'Configuration must contain unquoted KEY=value lines and comments.' >&2
    return 1 2>/dev/null || exit 1
  fi
  task_env_name=${BASH_REMATCH[1]}
  task_env_value=${BASH_REMATCH[2]}
  if [[ "$task_env_value" == \"* || "$task_env_value" == \'* ]]; then
    printf 'Use an unquoted value for %s.\n' "$task_env_name" >&2
    return 1 2>/dev/null || exit 1
  fi
  export "$task_env_name=$task_env_value"
done < "$task_env_path"
unset task_env_line task_env_name task_env_value task_env_path
