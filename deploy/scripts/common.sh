set -euo pipefail

export PATH="/usr/local/bin:/opt/homebrew/bin:$PATH"

DEPLOY_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [ ! -f "$DEPLOY_ROOT/deploy.env" ]; then
  echo "deploy.env is missing" >&2
  exit 1
fi

set -a
. "$DEPLOY_ROOT/deploy.env"
set +a

if ! command -v docker >/dev/null 2>&1; then
  echo "docker not found on this server" >&2
  exit 1
fi
