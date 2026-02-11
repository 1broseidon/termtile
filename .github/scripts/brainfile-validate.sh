#!/usr/bin/env bash
set -euo pipefail

BRAINFILE="${BRAINFILE:-.brainfile/brainfile.md}"

extract_frontmatter() {
  sed -n '/^---$/,/^---$/p' "$BRAINFILE" | sed '1d;$d'
}

fail() {
  echo "ERROR: $*"
  FAILURES=$((FAILURES + 1))
}

check_required_columns() {
  local frontmatter="$1"
  local required=("todo" "in-progress" "done" "backlog")

  for col in "${required[@]}"; do
    if ! echo "$frontmatter" | yq -r '.columns[].id' | grep -qx "$col"; then
      fail "Missing required column id: ${col}"
    fi
  done
}

validate_board() {
  local frontmatter="$1"
  local num_columns
  num_columns=$(echo "$frontmatter" | yq '.columns | length')

  declare -A seen_tasks=()

  for ((c = 0; c < num_columns; c++)); do
    local col_id
    col_id=$(echo "$frontmatter" | yq -r ".columns[${c}].id")

    local num_tasks
    num_tasks=$(echo "$frontmatter" | yq ".columns[${c}].tasks | length")

    for ((t = 0; t < num_tasks; t++)); do
      local task_path=".columns[${c}].tasks[${t}]"
      local task_id
      task_id=$(echo "$frontmatter" | yq -r "${task_path}.id")

      if [[ -z "$task_id" || "$task_id" == "null" ]]; then
        fail "Task missing id at ${task_path}"
        continue
      fi

      if [[ -n "${seen_tasks[$task_id]+x}" ]]; then
        fail "Duplicate task id detected: ${task_id}"
      fi
      seen_tasks["$task_id"]=1

      local status
      status=$(echo "$frontmatter" | yq -r "${task_path}.contract.status // \"\"")
      case "$status" in
        ""|ready|in_progress|delivered|done|failed) ;;
        *)
          fail "Task ${task_id} has unsupported contract.status=${status}"
          ;;
      esac

      if [[ "$status" == "in_progress" && "$col_id" != "in-progress" ]]; then
        fail "Task ${task_id} contract.status=in_progress but column=${col_id}"
      fi

      if [[ "$status" == "delivered" || "$status" == "done" ]]; then
        if [[ "$col_id" != "done" ]]; then
          fail "Task ${task_id} contract.status=${status} but column=${col_id} (expected done)"
        fi
      fi

      if [[ "$col_id" == "in-progress" && "$status" != "" && "$status" != "in_progress" ]]; then
        fail "Task ${task_id} is in-progress but contract.status=${status}"
      fi

      if [[ "$col_id" == "done" ]]; then
        if [[ "$status" == "ready" || "$status" == "in_progress" ]]; then
          fail "Task ${task_id} is done but contract.status=${status}"
        fi

        local subtask_count
        subtask_count=$(echo "$frontmatter" | yq "${task_path}.subtasks | length")
        if [[ "$subtask_count" -gt 0 ]]; then
          local incomplete_count
          incomplete_count=$(echo "$frontmatter" | yq "${task_path}.subtasks[] | select(.completed != true) | length" 2>/dev/null || true)
          # yq returns per-item lengths above; safer count with jq-style aggregation:
          incomplete_count=$(echo "$frontmatter" | yq "[${task_path}.subtasks[] | select(.completed != true)] | length")
          if [[ "$incomplete_count" -gt 0 ]]; then
            fail "Task ${task_id} is done but has ${incomplete_count} incomplete subtasks"
          fi
        fi
      fi
    done
  done
}

main() {
  if [[ ! -f "$BRAINFILE" ]]; then
    echo "ERROR: brainfile not found: $BRAINFILE"
    exit 1
  fi

  local frontmatter
  frontmatter=$(extract_frontmatter)
  if [[ -z "$frontmatter" ]]; then
    echo "ERROR: failed to extract YAML frontmatter from $BRAINFILE"
    exit 1
  fi

  FAILURES=0
  check_required_columns "$frontmatter"
  validate_board "$frontmatter"

  if [[ "$FAILURES" -gt 0 ]]; then
    echo "Brainfile validation failed with ${FAILURES} error(s)."
    exit 1
  fi

  echo "Brainfile validation passed."
}

main "$@"
