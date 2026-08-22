#!/usr/bin/env bash
. "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

if docker ps -a --format '{{.Names}}' | grep -qx "$CONTAINER_NAME"; then
  docker rm -f "$CONTAINER_NAME" >/dev/null
  echo "Removed the previous container"
fi

docker run -d \
  --name "$CONTAINER_NAME" \
  --restart unless-stopped \
  --network "$DOCKER_NETWORK" \
  --env-file "$DEPLOY_ROOT/sandrone.env" \
  --publish "$HOST_PORT:8080" \
  --label "org.opencontainers.image.revision=$REVISION" \
  "$IMAGE" >/dev/null

echo "Started container $CONTAINER_NAME on port $HOST_PORT"
