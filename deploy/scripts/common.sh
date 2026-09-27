set -euo pipefail

export PATH="/usr/local/bin:/opt/homebrew/bin:$PATH"

DEPLOY_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [ ! -f "$DEPLOY_ROOT/deploy.env" ]; then
  echo "deploy.env is missing" >&2
  exit 1
fi

set -a
. "$DEPLOY_ROOT/deploy.env"
set +a

if ! command -v docker >/dev/null 2>&1; then
  echo "docker not found on this server" >&2
  exit 1
fi

if ! docker compose version >/dev/null 2>&1; then
  echo "docker compose not found on this server" >&2
  exit 1
fi

compose_version="$(docker compose version --short)"
compose_version="${compose_version#v}"
compose_version="${compose_version%%-*}"
IFS=. read -r compose_major compose_minor _ <<< "$compose_version"
if [[ ! "${compose_major:-}" =~ ^[0-9]+$ ]] || [[ ! "${compose_minor:-}" =~ ^[0-9]+$ ]] || [ "$compose_major" -lt 2 ] || { [ "$compose_major" -eq 2 ] && [ "$compose_minor" -lt 39 ]; }; then
  echo "docker compose 2.39.0 or later is required, found: $compose_version" >&2
  exit 1
fi

if [ ! -f "$DEPLOY_ROOT/compose.yaml" ]; then
  echo "compose.yaml is missing" >&2
  exit 1
fi

run_compose() {
  docker compose \
    --project-name "$COMPOSE_PROJECT_NAME" \
    --env-file "$DEPLOY_ROOT/deploy.env" \
    --file "$DEPLOY_ROOT/compose.yaml" \
    "$@"
}

if ! run_compose config --quiet; then
  echo "Invalid deployment Compose configuration" >&2
  exit 1
fi

STOP_TIMEOUT="${STOP_TIMEOUT:-180}"

if [[ ! "$STOP_TIMEOUT" =~ ^[0-9]+$ ]] || [ "$STOP_TIMEOUT" -eq 0 ]; then
  echo "STOP_TIMEOUT must be a positive integer" >&2
  exit 1
fi

container_exists() {
  local names
  names="$(docker ps -a --format '{{.Names}}')" || return 2
  grep -Fqx "$1" <<< "$names"
}

container_running() {
  local names
  names="$(docker ps --format '{{.Names}}')" || return 2
  grep -Fqx "$1" <<< "$names"
}

stop_container() {
  local name="$1"
  if container_running "$name"; then
    docker stop --time "$STOP_TIMEOUT" "$name" >/dev/null
  fi
}

remove_container() {
  local name="$1"
  if container_exists "$name"; then
    stop_container "$name"
    docker rm "$name" >/dev/null
  fi
}

image_in_use() {
  local image_id="$1"
  local container_ids
  if ! container_ids="$(docker ps -aq --filter "ancestor=$image_id")"; then
    return 0
  fi
  [ -n "$container_ids" ]
}

cleanup_old_deploy_images() {
  local image_repository
  local current_tag
  local image_rows
  local repository
  local tag
  local image_id
  local reference

  image_repository="${IMAGE%:*}"
  current_tag="${IMAGE##*:}"
  if [ -z "$image_repository" ] || [ "$image_repository" = "$IMAGE" ] || [[ ! "$current_tag" =~ ^[0-9a-fA-F]{40}$ ]]; then
    echo "Refusing to clean images for an unexpected deployment reference: $IMAGE" >&2
    return 1
  fi
  if ! image_rows="$(docker image ls --no-trunc --format '{{.Repository}} {{.Tag}} {{.ID}}' "$image_repository")"; then
    echo "Failed to list deployment images for cleanup: $image_repository" >&2
    return 1
  fi
  while read -r repository tag image_id; do
    if [ -z "${repository:-}" ] || [ "$repository" != "$image_repository" ] || [[ ! "$tag" =~ ^[0-9a-fA-F]{40}$ ]]; then
      continue
    fi
    reference="$repository:$tag"
    if [ "$tag" = "$current_tag" ]; then
      echo "Kept current image: $reference"
      continue
    fi
    if image_in_use "$image_id"; then
      echo "Kept image used by a container: $reference"
      continue
    fi
    if docker image rm "$reference" >/dev/null; then
      echo "Removed old deployment image: $reference"
    else
      echo "Failed to remove old deployment image safely: $reference" >&2
    fi
  done <<< "$image_rows"
}
