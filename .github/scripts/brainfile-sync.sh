#!/usr/bin/env bash
set -euo pipefail

BRAINFILE=".brainfile/brainfile.md"
LABEL="brainfile"
ACTIVE_COLUMNS=("todo" "in-progress" "backlog")

# Extract YAML frontmatter (between --- markers)
extract_frontmatter() {
  sed -n '/^---$/,/^---$/p' "$BRAINFILE" | sed '1d;$d'
}

# Ensure the brainfile label exists
ensure_label() {
  if ! gh label list --json name -q '.[].name' | grep -qx "$LABEL"; then
    gh label create "$LABEL" --color "7057ff" --description "Synced from brainfile.md"
  fi
}

# Create a label if it doesn't exist (silently skip if it does)
ensure_extra_label() {
  local name="$1" color="$2"
  if ! gh label list --json name -q '.[].name' | grep -qx "$name"; then
    gh label create "$name" --color "$color" --description "" 2>/dev/null || true
  fi
}

# Build issue body from task data
build_body() {
  local task_id="$1" title="$2" description="$3" priority="$4" column_title="$5"
  local tags_json="$6" subtasks_json="$7" related_json="$8"

  local body="<!-- brainfile:${task_id} -->"$'\n'
  body+="**Priority:** ${priority:-none} | **Column:** ${column_title}"$'\n\n'

  if [[ -n "$description" && "$description" != "null" ]]; then
    # Unescape \n sequences in description
    body+="### Description"$'\n'
    body+="$(echo -e "$description")"$'\n\n'
  fi

  # Subtasks as checkboxes
  if [[ -n "$subtasks_json" && "$subtasks_json" != "null" && "$subtasks_json" != "[]" ]]; then
    body+="### Subtasks"$'\n'
    local count
    count=$(echo "$subtasks_json" | yq 'length')
    for ((i = 0; i < count; i++)); do
      local st_title st_completed
      st_title=$(echo "$subtasks_json" | yq ".[${i}].title")
      st_completed=$(echo "$subtasks_json" | yq ".[${i}].completed")
      if [[ "$st_completed" == "true" ]]; then
        body+="- [x] ${st_title}"$'\n'
      else
        body+="- [ ] ${st_title}"$'\n'
      fi
    done
    body+=$'\n'
  fi

  # Related files
  if [[ -n "$related_json" && "$related_json" != "null" && "$related_json" != "[]" ]]; then
    body+="### Related Files"$'\n'
    local count
    count=$(echo "$related_json" | yq 'length')
    for ((i = 0; i < count; i++)); do
      local rf
      rf=$(echo "$related_json" | yq ".[${i}]")
      body+="- \`${rf}\`"$'\n'
    done
    body+=$'\n'
  fi

  # Tags
  if [[ -n "$tags_json" && "$tags_json" != "null" && "$tags_json" != "[]" ]]; then
    local tags_str
    tags_str=$(echo "$tags_json" | yq '.[].tag // .' | tr '\n' ', ' | sed 's/,$//' | sed 's/,/, /g')
    body+="**Tags:** ${tags_str}"$'\n'
  fi

  echo "$body"
}

# Collect labels for a task
build_labels() {
  local priority="$1" column_id="$2" tags_json="$3"
  local labels="$LABEL"

  if [[ -n "$priority" && "$priority" != "null" ]]; then
    labels+=",priority:${priority}"
    case "$priority" in
      critical) ensure_extra_label "priority:critical" "b60205" ;;
      high)     ensure_extra_label "priority:high" "d93f0b" ;;
      medium)   ensure_extra_label "priority:medium" "fbca04" ;;
      low)      ensure_extra_label "priority:low" "0e8a16" ;;
    esac
  fi

  labels+=",${column_id}"
  ensure_extra_label "$column_id" "c5def5"

  if [[ -n "$tags_json" && "$tags_json" != "null" && "$tags_json" != "[]" ]]; then
    local count
    count=$(echo "$tags_json" | yq 'length')
    for ((i = 0; i < count; i++)); do
      local tag
      tag=$(echo "$tags_json" | yq ".[${i}]")
      labels+=",${tag}"
      ensure_extra_label "$tag" "ededed"
    done
  fi

  echo "$labels"
}

# Find issue number by brainfile marker
find_issue_by_task_id() {
  local task_id="$1"
  local marker="<!-- brainfile:${task_id} -->"
  # Search open issues with brainfile label for the marker
  gh issue list --label "$LABEL" --state open --json number,body -q \
    ".[] | select(.body | contains(\"${marker}\")) | .number"
}

main() {
  echo "==> Syncing brainfile to GitHub Issues"

  ensure_label

  local frontmatter
  frontmatter=$(extract_frontmatter)

  # Track which task IDs are active (in non-done columns)
  declare -A active_tasks

  # Get number of columns
  local num_columns
  num_columns=$(echo "$frontmatter" | yq '.columns | length')

  for ((c = 0; c < num_columns; c++)); do
    local col_id col_title
    col_id=$(echo "$frontmatter" | yq ".columns[${c}].id")
    col_title=$(echo "$frontmatter" | yq ".columns[${c}].title")

    # Check if this is an active column
    local is_active=false
    for ac in "${ACTIVE_COLUMNS[@]}"; do
      if [[ "$col_id" == "$ac" ]]; then
        is_active=true
        break
      fi
    done

    if [[ "$is_active" != "true" ]]; then
      echo "  Skipping column: ${col_title} (${col_id})"
      continue
    fi

    echo "  Processing column: ${col_title} (${col_id})"

    local num_tasks
    num_tasks=$(echo "$frontmatter" | yq ".columns[${c}].tasks | length")

    for ((t = 0; t < num_tasks; t++)); do
      local task_id task_title description priority tags subtasks related
      task_id=$(echo "$frontmatter" | yq ".columns[${c}].tasks[${t}].id")
      task_title=$(echo "$frontmatter" | yq ".columns[${c}].tasks[${t}].title")
      description=$(echo "$frontmatter" | yq ".columns[${c}].tasks[${t}].description // \"\"")
      priority=$(echo "$frontmatter" | yq ".columns[${c}].tasks[${t}].priority // \"\"")
      tags=$(echo "$frontmatter" | yq -o=json ".columns[${c}].tasks[${t}].tags // []")
      subtasks=$(echo "$frontmatter" | yq -o=json ".columns[${c}].tasks[${t}].subtasks // []")
      related=$(echo "$frontmatter" | yq -o=json ".columns[${c}].tasks[${t}].relatedFiles // []")

      active_tasks["$task_id"]=1
      echo "    Task: ${task_id} - ${task_title}"

      local issue_number
      issue_number=$(find_issue_by_task_id "$task_id")

      local body labels
      body=$(build_body "$task_id" "$task_title" "$description" "$priority" "$col_title" "$tags" "$subtasks" "$related")
      labels=$(build_labels "$priority" "$col_id" "$tags")

      if [[ -n "$issue_number" ]]; then
        echo "      Updating issue #${issue_number}"
        gh issue edit "$issue_number" \
          --title "${task_title}" \
          --body "$body" \
          --remove-label "" 2>/dev/null || true
        # Re-apply labels (handles column changes)
        gh issue edit "$issue_number" --add-label "$labels"
      else
        echo "      Creating new issue"
        gh issue create \
          --title "${task_title}" \
          --body "$body" \
          --label "$labels"
      fi
    done
  done

  # Close issues for tasks no longer in active columns
  echo "==> Checking for issues to close"
  local open_issues
  open_issues=$(gh issue list --label "$LABEL" --state open --json number,body -q '.[] | "\(.number)|\(.body)"')

  while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    local num="${line%%|*}"
    local issue_body="${line#*|}"

    # Extract task ID from marker
    local matched_id
    matched_id=$(echo "$issue_body" | grep -oP '<!-- brainfile:\K[^ ]+(?= -->)' || true)

    if [[ -n "$matched_id" && -z "${active_tasks[$matched_id]+x}" ]]; then
      echo "  Closing issue #${num} (${matched_id} moved to done or archived)"
      gh issue close "$num" --comment "Task ${matched_id} completed or archived in brainfile."
    fi
  done <<< "$open_issues"

  echo "==> Sync complete"
}

main "$@"
