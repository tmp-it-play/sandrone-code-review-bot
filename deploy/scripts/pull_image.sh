#!/usr/bin/env bash
. "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

echo "$GHCR_TOKEN" | docker login ghcr.io --username "$GHCR_USER" --password-stdin >/dev/null
echo "ghcr.io 로그인 완료"

docker pull "$IMAGE"
echo "이미지를 내려받았다: $IMAGE"

docker logout ghcr.io >/dev/null
echo "ghcr.io 로그아웃 완료"
