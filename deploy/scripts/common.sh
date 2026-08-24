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
if ! command -v timeout >/dev/null 2>&1; then
  echo "timeout not found on this server" >&2
  exit 1
fi

STOP_TIMEOUT="${STOP_TIMEOUT:-180}"
DOCKER_COMMAND_TIMEOUT="${DOCKER_COMMAND_TIMEOUT:-30}"

if [[ ! "$STOP_TIMEOUT" =~ ^[0-9]+$ ]] || [[ ! "$DOCKER_COMMAND_TIMEOUT" =~ ^[0-9]+$ ]] || [ "$STOP_TIMEOUT" -eq 0 ] || [ "$DOCKER_COMMAND_TIMEOUT" -eq 0 ]; then
  echo "deployment timeouts must be positive integers" >&2
  exit 1
fi

run_bounded() {
  local seconds="$1"
  shift
  timeout --signal=TERM --kill-after=10s "${seconds}s" "$@"
}

docker_bounded() {
  run_bounded "$DOCKER_COMMAND_TIMEOUT" docker "$@"
}

start_hook_watchdog() {
  local seconds="$1"
  (
    sleep "$seconds"
    kill -TERM "$$" 2>/dev/null || true
  ) </dev/null >/dev/null 2>&1 &
  HOOK_WATCHDOG_PID="$!"
}

stop_hook_watchdog() {
  if [ -n "${HOOK_WATCHDOG_PID:-}" ]; then
    kill "$HOOK_WATCHDOG_PID" >/dev/null 2>&1 || true
    wait "$HOOK_WATCHDOG_PID" >/dev/null 2>&1 || true
    HOOK_WATCHDOG_PID=""
  fi
}

container_exists() {
  local names
  names="$(docker_bounded ps -a --format '{{.Names}}')" || return 2
  grep -Fqx "$1" <<< "$names"
}

container_running() {
  local names
  names="$(docker_bounded ps --format '{{.Names}}')" || return 2
  grep -Fqx "$1" <<< "$names"
}

stop_container() {
  local name="$1"
  if container_running "$name"; then
    run_bounded "$((STOP_TIMEOUT + DOCKER_COMMAND_TIMEOUT))" docker stop --time "$STOP_TIMEOUT" "$name" >/dev/null
  fi
}

remove_container() {
  local name="$1"
  if container_exists "$name"; then
    stop_container "$name"
    docker_bounded rm "$name" >/dev/null
  fi
}

image_in_use() {
  local image_id="$1"
  local container_ids
  if ! container_ids="$(docker_bounded ps -aq --filter "ancestor=$image_id")"; then
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
  if ! image_rows="$(docker_bounded image ls --no-trunc --format '{{.Repository}} {{.Tag}} {{.ID}}' "$image_repository")"; then
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
    if docker_bounded image rm "$reference" >/dev/null; then
      echo "Removed old deployment image: $reference"
    else
      echo "Failed to remove old deployment image safely: $reference" >&2
    fi
  done <<< "$image_rows"
}

schedule_deploy_cleanup() {
  if [ "$DEPLOY_ROOT" != "/tmp/sandrone-deploy" ]; then
    echo "Refusing to remove unexpected deploy directory: $DEPLOY_ROOT" >&2
    return
  fi
  nohup sh -c "sleep 3; rm -rf -- '$DEPLOY_ROOT'" >/dev/null 2>&1 &
}
