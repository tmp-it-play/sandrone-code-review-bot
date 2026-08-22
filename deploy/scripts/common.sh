set -euo pipefail

export PATH="/usr/local/bin:/opt/homebrew/bin:$PATH"

DEPLOY_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [ ! -f "$DEPLOY_ROOT/deploy.env" ]; then
  echo "deploy.env가 없다" >&2
  exit 1
fi

set -a
. "$DEPLOY_ROOT/deploy.env"
set +a

if ! command -v docker >/dev/null 2>&1; then
  echo "서버에서 docker를 찾지 못했다" >&2
  exit 1
fi
