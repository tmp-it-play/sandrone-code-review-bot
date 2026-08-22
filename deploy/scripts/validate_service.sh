#!/usr/bin/env bash
. "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

healthy=0
for attempt in $(seq 1 30); do
  if docker exec "$CONTAINER_NAME" wget -qO- http://127.0.0.1:8080/healthz >/dev/null 2>&1; then
    healthy=1
    echo "헬스체크 통과 (${attempt}회차)"
    break
  fi
  sleep 2
done

if [ "$healthy" -ne 1 ]; then
  echo "헬스체크에 실패했다. 최근 로그:" >&2
  docker logs --tail 60 "$CONTAINER_NAME" >&2 || true
  docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
  docker image rm -f "$IMAGE" >/dev/null 2>&1 || true
  rm -rf "$DEPLOY_ROOT"
  exit 1
fi

docker image prune -f >/dev/null 2>&1 || true
echo "사용하지 않는 이미지를 정리했다"

nohup sh -c "sleep 3; rm -rf '$DEPLOY_ROOT'" >/dev/null 2>&1 &
echo "배포 파일을 정리하도록 예약했다: $DEPLOY_ROOT"
