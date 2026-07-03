#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
mkdir -p bin

_env_value() {
  ( set +e +o pipefail
    grep -E "^${1}=" "$ROOT/.env" 2>/dev/null \
      | head -1 \
      | cut -d= -f2-
  )
}

export BFF_PORT="${BFF_PORT:-$(_env_value BFF_PORT)}"
export BFF_PORT="${BFF_PORT:-8000}"
export BFF_WEB_UPSTREAM="${BFF_WEB_UPSTREAM:-$(_env_value BFF_WEB_UPSTREAM)}"
export BFF_WEB_UPSTREAM="${BFF_WEB_UPSTREAM:-http://127.0.0.1:8001}"
export BFF_AGENT_UPSTREAM="${BFF_AGENT_UPSTREAM:-$(_env_value BFF_AGENT_UPSTREAM)}"
export BFF_AGENT_UPSTREAM="${BFF_AGENT_UPSTREAM:-http://127.0.0.1:18080}"

go build -o ./bin/bff ./cmd/bff
exec ./bin/bff
