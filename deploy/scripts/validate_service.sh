#!/usr/bin/env bash
. "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

HEALTH_URL="http://127.0.0.1:8080${BASE_PATH:-}/healthz"

healthy=0
for attempt in $(seq 1 30); do
  if docker exec "$CONTAINER_NAME" wget -qO- "$HEALTH_URL" >/dev/null 2>&1; then
    healthy=1
    echo "Health check passed: $HEALTH_URL (attempt ${attempt})"
    break
  fi
  sleep 2
done

if [ "$healthy" -ne 1 ]; then
  echo "Health check failed: $HEALTH_URL" >&2
  echo "Recent logs:" >&2
  docker logs --tail 60 "$CONTAINER_NAME" >&2 || true
  docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
  docker image rm -f "$IMAGE" >/dev/null 2>&1 || true
  rm -rf "$DEPLOY_ROOT"
  exit 1
fi

docker image prune -f >/dev/null 2>&1 || true
echo "Pruned dangling images"

nohup sh -c "sleep 3; rm -rf '$DEPLOY_ROOT'" >/dev/null 2>&1 &
echo "Scheduled cleanup of the deploy directory: $DEPLOY_ROOT"
