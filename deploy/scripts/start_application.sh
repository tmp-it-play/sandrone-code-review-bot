#!/usr/bin/env bash
. "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

START_ATTEMPTED=0

cleanup_failed_start() {
  status="$?"
  trap - EXIT TERM HUP INT
  if [ "$status" -ne 0 ]; then
    if [ "$START_ATTEMPTED" -eq 1 ]; then
      docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
    fi
    schedule_deploy_cleanup
  fi
  exit "$status"
}
trap cleanup_failed_start EXIT
trap 'exit 124' TERM HUP INT

remove_container "$CONTAINER_NAME"
START_ATTEMPTED=1

if ! docker run -d \
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
bash "$DEPLOY_ROOT/scripts/validate_service.sh"
