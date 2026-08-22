#!/usr/bin/env bash
. "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

echo "$GHCR_TOKEN" | docker login ghcr.io --username "$GHCR_USER" --password-stdin >/dev/null
echo "Logged in to ghcr.io"

docker pull "$IMAGE"
echo "Pulled image: $IMAGE"

docker logout ghcr.io >/dev/null
echo "Logged out of ghcr.io"
