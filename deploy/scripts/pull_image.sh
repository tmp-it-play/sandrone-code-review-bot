#!/usr/bin/env bash
. "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

logout() {
  docker logout ghcr.io >/dev/null 2>&1 || true
}

finish() {
  status="$?"
  trap - EXIT TERM HUP INT
  logout
  if [ "$status" -ne 0 ]; then
    schedule_deploy_cleanup
  fi
  exit "$status"
}
trap finish EXIT
trap 'exit 124' TERM HUP INT

docker login ghcr.io --username "$GHCR_USER" --password-stdin <<< "$GHCR_TOKEN" >/dev/null
echo "Logged in to ghcr.io"

docker pull "$IMAGE"
echo "Pulled image: $IMAGE"

logout

if ! docker model status >/dev/null 2>&1; then
  echo "Docker Model Runner is unavailable" >&2
  exit 1
fi

models="$(run_compose config --models)"
if [ -z "$models" ]; then
  echo "No deployment model is configured" >&2
  exit 1
fi

while IFS= read -r model; do
  if [ -z "$model" ]; then
    continue
  fi
  docker model pull "$model"
  docker model inspect "$model" >/dev/null
  echo "Pulled model: $model"
done <<< "$models"

trap - EXIT TERM HUP INT
echo "Logged out of ghcr.io"
