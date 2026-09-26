#!/usr/bin/env bash
. "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

cleanup_validation() {
  status="$?"
  trap - EXIT TERM HUP INT
  if [ "$status" -ne 0 ]; then
    docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
  fi
  exit "$status"
}
trap cleanup_validation EXIT
trap 'exit 124' TERM HUP INT

HEALTH_URL="http://127.0.0.1:8080${BASE_PATH:-}/healthz"

healthy=0
for ((attempt = 1; attempt <= 30; attempt++)); do
  if docker exec "$CONTAINER_NAME" wget -T 5 -qO- "$HEALTH_URL" >/dev/null 2>&1; then
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
  exit 1
fi

if ! cleanup_old_deploy_images; then
  echo "Deployment image cleanup did not complete" >&2
fi
