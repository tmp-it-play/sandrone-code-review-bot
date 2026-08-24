#!/usr/bin/env bash
. "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

cleanup_failed_start() {
  status="$?"
  trap - EXIT TERM HUP INT
  stop_hook_watchdog
  if [ "$status" -ne 0 ]; then
    schedule_deploy_cleanup
  fi
  exit "$status"
}
trap cleanup_failed_start EXIT
trap 'exit 124' TERM HUP INT
start_hook_watchdog 420

remove_container "$CONTAINER_NAME"

if ! run_bounded 90 docker run -d \
    --name "$CONTAINER_NAME" \
    --restart unless-stopped \
    --network "$DOCKER_NETWORK" \
    --env-file "$DEPLOY_ROOT/sandrone.env" \
    --publish "$HOST_PORT:8080" \
    --label "org.opencontainers.image.revision=$REVISION" \
    "$IMAGE" >/dev/null; then
  echo "Failed to start the new container" >&2
  exit 1
fi

echo "Started container $CONTAINER_NAME on port $HOST_PORT"
stop_hook_watchdog
trap - EXIT TERM HUP INT
exec bash "$DEPLOY_ROOT/scripts/validate_service.sh"
