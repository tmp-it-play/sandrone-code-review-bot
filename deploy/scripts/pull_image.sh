#!/usr/bin/env bash
. "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

logout() {
  docker_bounded logout ghcr.io >/dev/null 2>&1 || true
}

finish() {
  status="$?"
  trap - EXIT TERM HUP INT
  stop_hook_watchdog
  logout
  if [ "$status" -ne 0 ]; then
    schedule_deploy_cleanup
  fi
  exit "$status"
}
trap finish EXIT
trap 'exit 124' TERM HUP INT
start_hook_watchdog 870

echo "$GHCR_TOKEN" | run_bounded 60 docker login ghcr.io --username "$GHCR_USER" --password-stdin >/dev/null
echo "Logged in to ghcr.io"

run_bounded 840 docker pull "$IMAGE"
echo "Pulled image: $IMAGE"

logout
stop_hook_watchdog
trap - EXIT TERM HUP INT
echo "Logged out of ghcr.io"
