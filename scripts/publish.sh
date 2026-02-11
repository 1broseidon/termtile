#!/usr/bin/env bash
set -euo pipefail

# Release configuration
REMOTE="${REMOTE:-origin}"
TAG_PREFIX="${TAG_PREFIX:-v}"
BUMP="${BUMP:-}"
VERSION="${VERSION:-}"
DRY_RUN="${DRY_RUN:-0}"
ALLOW_DIRTY="${ALLOW_DIRTY:-0}"
RUN_TESTS="${RUN_TESTS:-1}"
VERSION_FILE="internal/mcp/server.go"

usage() {
  cat <<'EOF'
Usage:
  make publish [BUMP=patch|minor|major|none]
  make publish VERSION=vX.Y.Z

Optional env:
  REMOTE=<git remote>         default: origin
  TAG_PREFIX=<prefix>         default: v
  RUN_TESTS=0|1               default: 1
  DRY_RUN=0|1                 default: 0
  ALLOW_DIRTY=0|1             default: 0
EOF
}

run() {
  if [[ "${DRY_RUN}" == "1" ]]; then
    printf '[dry-run] %q' "$1"
    shift
    for arg in "$@"; do
      printf ' %q' "$arg"
    done
    printf '\n'
    return 0
  fi
  "$@"
}

require_repo_root() {
  local root
  root="$(git rev-parse --show-toplevel)"
  cd "${root}"
}

validate_clean_tree() {
  if [[ "${ALLOW_DIRTY}" == "1" ]]; then
    return 0
  fi
  if ! git diff --quiet || ! git diff --cached --quiet; then
    echo "Refusing to publish with uncommitted changes. Commit/stash first, or set ALLOW_DIRTY=1." >&2
    exit 1
  fi
}

validate_bump() {
  case "${BUMP}" in
    patch|minor|major|none) ;;
    *)
      echo "Invalid BUMP=${BUMP}. Expected patch|minor|major|none." >&2
      usage
      exit 1
      ;;
  esac
}

prompt_bump_choice() {
  local choice
  printf "Select version bump [major/minor/patch/none] (default: patch): "
  read -r choice || true
  choice="$(printf '%s' "${choice}" | tr '[:upper:]' '[:lower:]' | xargs)"
  case "${choice}" in
    ""|p|patch) BUMP="patch" ;;
    m|minor) BUMP="minor" ;;
    maj|major) BUMP="major" ;;
    n|none) BUMP="none" ;;
    *)
      echo "Invalid selection: ${choice}" >&2
      exit 1
      ;;
  esac
}

resolve_bump() {
  # VERSION overrides bump choice entirely.
  if [[ -n "${VERSION}" ]]; then
    return 0
  fi

  if [[ -n "${BUMP}" ]]; then
    validate_bump
    return 0
  fi

  if [[ -t 0 && -t 1 ]]; then
    prompt_bump_choice
    validate_bump
    return 0
  fi

  # Non-interactive fallback keeps old behavior deterministic.
  BUMP="patch"
  echo "BUMP not provided in non-interactive mode; defaulting to patch."
}

latest_version_tag() {
  git tag -l "${TAG_PREFIX}[0-9]*.[0-9]*.[0-9]*" --sort=-v:refname | head -n1 || true
}

normalize_tag() {
  local input="$1"
  if [[ "${input}" == "${TAG_PREFIX}"* ]]; then
    printf '%s\n' "${input}"
  else
    printf '%s%s\n' "${TAG_PREFIX}" "${input}"
  fi
}

ensure_semver_tag() {
  local tag="$1"
  local re="^${TAG_PREFIX}[0-9]+\\.[0-9]+\\.[0-9]+$"
  if [[ ! "${tag}" =~ ${re} ]]; then
    echo "Invalid VERSION/tag: ${tag}. Expected ${TAG_PREFIX}X.Y.Z" >&2
    exit 1
  fi
}

next_tag_from_bump() {
  local latest="$1"
  local major=0 minor=0 patch=0
  if [[ -n "${latest}" ]]; then
    local raw="${latest#${TAG_PREFIX}}"
    IFS='.' read -r major minor patch <<<"${raw}"
  fi

  case "${BUMP}" in
    patch) patch=$((patch + 1)) ;;
    minor)
      minor=$((minor + 1))
      patch=0
      ;;
    major)
      major=$((major + 1))
      minor=0
      patch=0
      ;;
  esac

  printf '%s%d.%d.%d\n' "${TAG_PREFIX}" "${major}" "${minor}" "${patch}"
}

update_version_file() {
  local numeric_version="$1"
  if [[ ! -f "${VERSION_FILE}" ]]; then
    echo "Version file not found: ${VERSION_FILE}" >&2
    exit 1
  fi

  if [[ "${DRY_RUN}" == "1" ]]; then
    echo "[dry-run] update ${VERSION_FILE}: ServerVersion -> ${numeric_version}"
    return 0
  fi

  local tmp
  tmp="$(mktemp)"
  sed -E "s/(ServerVersion[[:space:]]*=[[:space:]]*\")[^\"]+(\")/\\1${numeric_version}\\2/" "${VERSION_FILE}" > "${tmp}"
  mv "${tmp}" "${VERSION_FILE}"
}

main() {
  require_repo_root
  validate_clean_tree
  resolve_bump

  local tag
  local numeric
  if [[ -n "${VERSION}" ]]; then
    run git fetch --tags "${REMOTE}"
    tag="$(normalize_tag "${VERSION}")"
    ensure_semver_tag "${tag}"
    numeric="${tag#${TAG_PREFIX}}"
  else
    if [[ "${BUMP}" == "none" ]]; then
      echo "Publishing current branch without version bump/tag."
      if [[ "${RUN_TESTS}" == "1" ]]; then
        run go test ./...
      fi
      run git push "${REMOTE}" HEAD
      echo "Published branch HEAD (no release tag)."
      return 0
    fi

    run git fetch --tags "${REMOTE}"
    local latest
    latest="$(latest_version_tag)"
    tag="$(next_tag_from_bump "${latest}")"
    ensure_semver_tag "${tag}"
    numeric="${tag#${TAG_PREFIX}}"
  fi

  if git rev-parse -q --verify "refs/tags/${tag}" >/dev/null; then
    echo "Tag already exists: ${tag}" >&2
    exit 1
  fi

  echo "Publishing ${tag} (ServerVersion=${numeric})"

  update_version_file "${numeric}"

  if [[ "${RUN_TESTS}" == "1" ]]; then
    run go test ./...
  fi

  run git add "${VERSION_FILE}"
  if ! git diff --cached --quiet; then
    run git commit -m "release: ${tag}"
  else
    echo "No version-file changes to commit; proceeding to tag current HEAD."
  fi

  run git tag -a "${tag}" -m "Release ${tag}"
  run git push "${REMOTE}" HEAD
  run git push "${REMOTE}" "${tag}"

  echo "Published ${tag}"
}

main "$@"
