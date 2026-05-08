#!/usr/bin/env bash

set -euo pipefail

readonly VERSION_INPUT="${1:-}"
readonly DOCKERFILE="Dockerfile"

log() {
  echo "[update-go-version] $*"
}

die() {
  echo "[update-go-version] ERROR: $*" >&2
  exit 1
}

require_file() {
  local file="$1"
  [[ -f "$file" ]] || die "Required file not found: $file"
}

require_cmd() {
  local cmd="$1"
  command -v "$cmd" >/dev/null 2>&1 || die "Required command not found: $cmd"
}

validate_version() {
  [[ "$VERSION_INPUT" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "Invalid Go version '$VERSION_INPUT'. Expected format: MAJOR.MINOR.PATCH"
}

replace_or_fail() {
  local file="$1"
  local match_regex="$2"
  local sed_expr="$3"

  grep -Eq "$match_regex" "$file" || die "Expected pattern not found in $file"
  sed -E -i "$sed_expr" "$file"
}

update_known_version_files() {
  log "Updating Go version references in known files"

  replace_or_fail "go.mod" '^go [0-9]+\.[0-9]+\.[0-9]+$' "s/^go [0-9]+\.[0-9]+\.[0-9]+$/go ${VERSION_INPUT}/"
  replace_or_fail "go.tool.mod" '^go [0-9]+\.[0-9]+\.[0-9]+$' "s/^go [0-9]+\.[0-9]+\.[0-9]+$/go ${VERSION_INPUT}/"
  replace_or_fail ".golangci.yaml" '^[[:space:]]*go:[[:space:]]*[0-9]+\.[0-9]+(\.[0-9]+)?[[:space:]]*$' "s/^([[:space:]]*go:[[:space:]]*).*/\\1${VERSION_INPUT}/"

  require_file ".mise.toml"
  awk -v version="$VERSION_INPUT" '
    BEGIN { in_tools = 0; updated = 0 }
    {
      if ($0 ~ /^\[tools\][[:space:]]*$/) {
        in_tools = 1
        print
        next
      }
      if (in_tools && $0 ~ /^\[[^]]+\][[:space:]]*$/) {
        in_tools = 0
      }
      if (in_tools && !updated && $0 ~ /^[[:space:]]*(go|golang)[[:space:]]*=[[:space:]]*"[^"]+"[[:space:]]*$/) {
        sub(/"[^"]+"/, "\"" version "\"")
        updated = 1
      }
      print
    }
    END {
      if (!updated) {
        print "missing go/golang entry in [tools] section of .mise.toml" > "/dev/stderr"
        exit 1
      }
    }
  ' .mise.toml > .mise.toml.tmp && mv .mise.toml.tmp .mise.toml

  if [[ -f "mise.toml" ]]; then
    log "Updating optional mise.toml"
    sed -E -i "s/^([[:space:]]*(go|golang)[[:space:]]*=[[:space:]]*)\"[^\"]+\"[[:space:]]*$/\\1\"${VERSION_INPUT}\"/" mise.toml || true
  fi
}

extract_repo_and_stage_from_dockerfile() {
  local from_line
  from_line="$(awk '/^FROM[[:space:]]+/ { print; exit }' "$DOCKERFILE")"
  [[ -n "$from_line" ]] || die "No FROM line found in $DOCKERFILE"

  local image_ref
  image_ref="$(printf '%s\n' "$from_line" | sed -E 's/^FROM[[:space:]]+([^[:space:]]+).*/\1/')"
  local stage_suffix
  stage_suffix="$(printf '%s\n' "$from_line" | sed -nE 's/^FROM[[:space:]]+[^[:space:]]+([[:space:]]+AS[[:space:]]+.+)$/\1/p')"

  [[ "$image_ref" =~ ^([^@]+)@sha256:[a-f0-9]{64}$ ]] || die "Expected digest-pinned base image in $DOCKERFILE"
  local image_with_tag="${BASH_REMATCH[1]}"
  local repo="${image_with_tag%:*}"
  [[ "$repo" != "$image_with_tag" ]] || die "Unable to parse repo and tag from Dockerfile FROM line"

  printf '%s\t%s\n' "$repo" "$stage_suffix"
}

find_latest_dated_tag() {
  local repo="$1"
  local ns_repo="$repo"
  if [[ "$ns_repo" =~ ^docker\.io/(.+/.+)$ ]]; then
    ns_repo="${BASH_REMATCH[1]}"
  fi
  [[ "$ns_repo" =~ ^[^./]+/[^/]+$ ]] || die "Docker Hub lookup expects repo in namespace/name form, got: $repo"

  local namespace="${ns_repo%/*}"
  local repository="${ns_repo#*/}"
  local api_url="https://hub.docker.com/v2/namespaces/${namespace}/repositories/${repository}/tags?page_size=100"
  local matches=""

  while [[ -n "$api_url" && "$api_url" != "null" ]]; do
    local body
    body="$(curl -fsSL "$api_url")"

    local page
    page="$(printf '%s\n' "$body" | grep -oE '"name"[[:space:]]*:[[:space:]]*"[^"]+"' | sed -E 's/^"name"[[:space:]]*:[[:space:]]*"([^"]+)"$/\1/' | grep -E "^${VERSION_INPUT}-[0-9]{4}-[0-9]{2}-[0-9]{2}$" || true)"
    if [[ -n "$page" ]]; then
      matches+=$'\n'
      matches+="$page"
    fi

    api_url="$(printf '%s\n' "$body" | tr -d '\n' | sed -nE 's/.*"next"[[:space:]]*:[[:space:]]*"([^"]*)".*/\1/p')"
    [[ -n "$api_url" ]] || api_url="null"
  done

  local latest
  latest="$(printf '%s\n' "$matches" | sed '/^[[:space:]]*$/d' | sort -u | tail -n 1)"
  [[ -n "$latest" ]] || die "No Docker Hub dated tag found for ${repo} with version ${VERSION_INPUT}"
  printf '%s\n' "$latest"
}

resolve_digest_with_docker() {
  local image_ref="$1"
  local digest=""

  local inspect_output
  inspect_output="$(docker buildx imagetools inspect "$image_ref" 2>/dev/null || true)"
  if [[ "$inspect_output" =~ Digest:[[:space:]]*(sha256:[a-f0-9]{64}) ]]; then
    digest="${BASH_REMATCH[1]}"
  fi

  if [[ -z "$digest" ]]; then
    docker pull "$image_ref" >/dev/null 2>&1 || true
    local repo_digest
    repo_digest="$(docker image inspect --format '{{index .RepoDigests 0}}' "$image_ref" 2>/dev/null || true)"
    if [[ "$repo_digest" == *@sha256:* ]]; then
      digest="${repo_digest##*@}"
    fi
  fi

  [[ "$digest" =~ ^sha256:[a-f0-9]{64}$ ]] || die "Failed to resolve digest for $image_ref via docker"
  printf '%s\n' "$digest"
}

update_dockerfile_base_image() {
  local parsed
  parsed="$(extract_repo_and_stage_from_dockerfile)"
  local repo="${parsed%%$'\t'*}"
  local stage_suffix="${parsed#*$'\t'}"

  local tag
  tag="$(find_latest_dated_tag "$repo")"
  local image_ref="${repo}:${tag}"
  log "Resolving digest for $image_ref"
  local digest
  digest="$(resolve_digest_with_docker "$image_ref")"

  local replacement="FROM ${image_ref}@${digest}${stage_suffix}"
  awk -v line="$replacement" 'BEGIN { done = 0 } { if (!done && $0 ~ /^FROM[[:space:]]+/) { print line; done = 1 } else { print } } END { if (!done) { exit 1 } }' "$DOCKERFILE" > "$DOCKERFILE.tmp" && mv "$DOCKERFILE.tmp" "$DOCKERFILE"
}

show_detected_go_version_refs() {
  log "Scanning tracked files for Go version keys"
  grep -HnE '(^go [0-9]+\.[0-9]+\.[0-9]+$|^[[:space:]]*go:[[:space:]]*[0-9]+\.[0-9]+(\.[0-9]+)?[[:space:]]*$|^[[:space:]]*(go|golang)[[:space:]]*=[[:space:]]*"[^"]+"[[:space:]]*$)' go.mod go.tool.mod .golangci.yaml .mise.toml 2>/dev/null || true
  if [[ -f "mise.toml" ]]; then
    grep -HnE '^[[:space:]]*(go|golang)[[:space:]]*=[[:space:]]*"[^"]+"[[:space:]]*$' mise.toml 2>/dev/null || true
  fi
}

main() {
  validate_version
  require_file "go.mod"
  require_file "go.tool.mod"
  require_file ".golangci.yaml"
  require_file ".mise.toml"
  require_file "$DOCKERFILE"
  require_cmd grep
  require_cmd sed
  require_cmd awk
  require_cmd curl
  require_cmd docker

  # Preflight Docker resolution before mutating files.
  local preflight
  preflight="$(extract_repo_and_stage_from_dockerfile)"
  local preflight_repo="${preflight%%$'\t'*}"
  local preflight_tag
  preflight_tag="$(find_latest_dated_tag "$preflight_repo")"
  resolve_digest_with_docker "${preflight_repo}:${preflight_tag}" >/dev/null

  show_detected_go_version_refs
  update_known_version_files
  update_dockerfile_base_image
  log "Done. Updated Go references to $VERSION_INPUT"
}

main
