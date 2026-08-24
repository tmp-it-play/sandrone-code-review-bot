#!/usr/bin/env bash
. "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

cleanup_validation() {
  status="$?"
  trap - EXIT TERM HUP INT
  stop_hook_watchdog
  schedule_deploy_cleanup
  exit "$status"
}
trap cleanup_validation EXIT
trap 'exit 124' TERM HUP INT
start_hook_watchdog 600

HEALTH_URL="http://127.0.0.1:8080${BASE_PATH:-}/healthz"

healthy=0
for attempt in $(seq 1 30); do
  if run_bounded 5 docker exec "$CONTAINER_NAME" wget -qO- "$HEALTH_URL" >/dev/null 2>&1; then
    healthy=1
    echo "Health check passed: $HEALTH_URL (attempt ${attempt})"
    break
  fi
  sleep 2
done

if [ "$healthy" -ne 1 ]; then
  echo "Health check failed: $HEALTH_URL" >&2
  echo "Recent logs:" >&2
  docker_bounded logs --tail 60 "$CONTAINER_NAME" >&2 || true
  remove_container "$CONTAINER_NAME" || true
  exit 1
fi

if ! cleanup_old_deploy_images; then
  echo "Deployment image cleanup did not complete" >&2
fi

echo "Scheduled cleanup of the deploy directory: $DEPLOY_ROOT"
