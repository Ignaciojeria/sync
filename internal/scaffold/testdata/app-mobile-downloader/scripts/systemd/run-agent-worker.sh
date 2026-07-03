#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
mkdir -p tmp

_env_value() {
  ( set +e +o pipefail
    grep -E "^${1}=" "$ROOT/.env" 2>/dev/null \
      | head -1 \
      | cut -d= -f2-
  )
}

export AGENT_WORKER_PORT="${AGENT_WORKER_PORT:-127.0.0.1:18080}"
export OIDC_JWKS_URI="${OIDC_JWKS_URI:-$(_env_value OIDC_JWKS_URI)}"
export JWKS_URL="${JWKS_URL:-${OIDC_JWKS_URI:-}}"
export JWT_HMAC_SECRET="${JWT_HMAC_SECRET:-}"
export OIDC_ISSUER="${OIDC_ISSUER:-$(_env_value OIDC_ISSUER)}"
export OIDC_CLIENT_ID="${OIDC_CLIENT_ID:-$(_env_value OIDC_CLIENT_ID)}"
export JWT_AUDIENCE="${JWT_AUDIENCE:-$(_env_value JWT_AUDIENCE)}"

go build -o ./tmp/agent-worker ./cmd/agent-worker
exec ./tmp/agent-worker
