#!/usr/bin/env bash
# scripts/run-all.sh — levanta los 3 procesos del boilerplate en
# orden: BFF (gateway), web-server, agent-worker. La idea es tener un
# solo comando para arrancar todo en dev y validar la topología §11 de
# doc/agent-runtime.md.
#
# Uso:
#   scripts/run-all.sh start   # construye (si hace falta) y arranca los 3
#   scripts/run-all.sh stop    # mata los 3 procesos
#   scripts/run-all.sh status  # muestra pidfile y reachability
#
# Variables de entorno relevantes (con defaults):
#   BFF_PORT              = 8000
#   BFF_WEB_UPSTREAM      = http://127.0.0.1:8001
#   BFF_AGENT_UPSTREAM    = http://127.0.0.1:18080
#   WEB_PORT              = 8001
#   AGENT_WORKER_PORT     = 18080
#   AGENT_WORKER_HOST     = 127.0.0.1
#
# Los pids se persisten en tmp/run/*.pid para stop limpio.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PIDDIR="$ROOT/tmp/run"
mkdir -p "$PIDDIR"

BFF_PORT="${BFF_PORT:-8000}"
BFF_WEB_UPSTREAM="${BFF_WEB_UPSTREAM:-http://127.0.0.1:8001}"
BFF_AGENT_UPSTREAM="${BFF_AGENT_UPSTREAM:-http://127.0.0.1:18080}"
WEB_PORT="${WEB_PORT:-8001}"
AGENT_WORKER_PORT="${AGENT_WORKER_PORT:-18080}"
AGENT_WORKER_HOST="${AGENT_WORKER_HOST:-127.0.0.1}"

# Homologación Opción A (AGENTS.md §14): si existe .env, leemos las
# variables OIDC_/JWT_ de ahi. Asi el worker valida JWT contra el mismo
# JWKS que el web, sin secretos compartidos. Si JWKS_URL no termina
# resultando en un valor, el worker falla loud (loadKeyfunc requiere
# JWKS_URL o JWT_HMAC_SECRET).
_env_value() {
  # Subshell con errexit/pipefail desactivados localmente: si la clave
  # no existe en .env, grep sale 1 y propagaría el exit al padre bajo
  # set -uo pipefail. Aca lo silenciamos y devolvemos cadena vacía.
  ( set +e +o pipefail
    grep -E "^${1}=" "$ROOT/.env" 2>/dev/null \
      | head -1 \
      | cut -d= -f2- \
      | sed -E "s/^['\"]|['\"]$//g"
  )
}
if [ -f "$ROOT/.env" ]; then
  OIDC_JWKS_URI="${OIDC_JWKS_URI:-$(_env_value OIDC_JWKS_URI)}"
  OIDC_ISSUER="${OIDC_ISSUER:-$(_env_value OIDC_ISSUER)}"
  OIDC_CLIENT_ID="${OIDC_CLIENT_ID:-$(_env_value OIDC_CLIENT_ID)}"
  JWT_AUDIENCE="${JWT_AUDIENCE:-$(_env_value JWT_AUDIENCE)}"
fi

cmd="${1:-start}"

build_binaries() {
  echo "+ go build -o $ROOT/tmp/web ./cmd/api"
  ( cd "$ROOT" && go build -o ./tmp/web ./cmd/api )
  echo "+ go build -o $ROOT/tmp/agent-worker ./cmd/agent-worker"
  ( cd "$ROOT" && go build -o ./tmp/agent-worker ./cmd/agent-worker )
  echo "+ go build -o $ROOT/bin/bff ./cmd/bff"
  mkdir -p "$ROOT/bin"
  ( cd "$ROOT" && go build -o ./bin/bff ./cmd/bff )
}

start_one() {
  local name="$1"
  shift
  local pidfile="$PIDDIR/$name.pid"
  if [ -f "$pidfile" ] && kill -0 "$(cat "$pidfile")" 2>/dev/null; then
    echo "$name ya está corriendo (pid $(cat "$pidfile"))"
    return
  fi
  echo "+ start $name: $*"
  # bash -c exec reemplaza la shell con el binario: el bash_pid
  # es transitorio; usamos el PID del binario directamente.
  export LOG_OUT="$PIDDIR/$name.log"
  bash -c 'exec "$@">"$LOG_OUT" 2>&1' -- "$@" </dev/null &
  echo $! > "$pidfile"
  unset LOG_OUT
}

start() {
  build_binaries

  # Worker consume el mismo JWKS que el web-server (.env OIDC_JWKS_URI,
  # exportado como JWKS_URL). HMAC queda opt-in: sólo se usa si el
  # operador lo setea explícitamente. Sin ninguno de los dos, el worker
  # falla loud al arrancar (loadKeyfunc lo rechaza).
  #
  # JWT_AUDIENCE queda VACÍO por default para que el worker's middleware
  # use FirstNonEmpty(JWTAudience, OIDCClientID) → OIDCClientID. Si
  # por el contrario se le inyecta un JWTAudience literal (como
  # "dev-only-aud"), el worker exige que el aud del JWT coincida y
  # rechaza los tokens de Casdoor cuyo aud es el client_id.
  JWKS_URL="${JWKS_URL:-$OIDC_JWKS_URI}"
  JWT_HMAC_SECRET="${JWT_HMAC_SECRET:-}"  # vacío por default
  OIDC_ISSUER="${OIDC_ISSUER:-https://dev-only.invalid}"
  OIDC_CLIENT_ID="${OIDC_CLIENT_ID:-}"
  JWT_AUDIENCE="${JWT_AUDIENCE:-}"
  env "AGENT_WORKER_PORT=$AGENT_WORKER_HOST:$AGENT_WORKER_PORT" \
         "JWKS_URL=$JWKS_URL" \
         "JWT_HMAC_SECRET=$JWT_HMAC_SECRET" \
         "OIDC_ISSUER=$OIDC_ISSUER" \
         "OIDC_CLIENT_ID=$OIDC_CLIENT_ID" \
         "JWT_AUDIENCE=$JWT_AUDIENCE" \
         "$ROOT/tmp/agent-worker" &
  WORKER_PID=$!
  echo "$WORKER_PID" > "$PIDDIR/worker.pid"
  echo "  worker pid $WORKER_PID (JWKS_URL=${JWKS_URL:-<none>}${JWT_HMAC_SECRET:+, HMAC fallback})"

  # Pre-check: si :WEB_PORT ya está ocupado por algo fuera del
  # orquestador (ej. un `go run .` previo), abortamos antes de que
  # el web-server panice con "bind: address already in use". No
  # intentamos matarlo: podría ser de otro proyecto.
  if ss -ltn 2>/dev/null | grep -qE ":${WEB_PORT}[[:space:]]"; then
    holder=$(ss -ltnp 2>/dev/null | grep -E ":${WEB_PORT} " | grep -oE 'pid=[0-9]+' | head -1 || true)
    echo "❌  :${WEB_PORT} ya está en uso ${holder:+($holder)}." >&2
    echo "    scripts/run-all.sh stop no lo mata (no está bajo tmp/run/*.pid)." >&2
    echo "    Liberá el puerto o cambiá WEB_PORT y reintentá." >&2
    exit 1
  fi

  # Web-server en :8001, detrás del BFF. air también lo compila y lo
  # corre bajo hot-reload; este script es la vía manual/orquestada.
  start_one web \
    env PORT="$WEB_PORT" \
    "$ROOT/tmp/web"

  # BFF al frente, en :8000 por default. Es el que ve Internet.
  start_one bff \
    env BFF_PORT="$BFF_PORT" \
       BFF_WEB_UPSTREAM="$BFF_WEB_UPSTREAM" \
       BFF_AGENT_UPSTREAM="$BFF_AGENT_UPSTREAM" \
    "$ROOT/bin/bff"

  echo
  echo
  echo "Topología:"
  echo "  browser  ─►  bff :${BFF_PORT}              (gateway estable)"
  echo "  bff      ─►  web-server :$WEB_PORT          (recibe hot-reload por air)"
  echo "  bff      ─►  agent-worker ${AGENT_WORKER_HOST}:${AGENT_WORKER_PORT}  (recibe lifecycle propio)"
  echo
  echo "Probad:"
  echo "  curl -i http://127.0.0.1:$BFF_PORT/agent/healthz"
  echo "  curl -i http://127.0.0.1:$BFF_PORT/"
}

stop() {
  for name in bff web worker; do
    pidfile="$PIDDIR/$name.pid"
    if [ -f "$pidfile" ]; then
      pid="$(cat "$pidfile")"
      if kill -0 "$pid" 2>/dev/null; then
        echo "+ stop $name (pid $pid)"
        kill "$pid" || true
      fi
      rm -f "$pidfile"
    fi
  done
}

status() {
  for name in bff web worker; do
    pidfile="$PIDDIR/$name.pid"
    if [ -f "$pidfile" ] && kill -0 "$(cat "$pidfile")" 2>/dev/null; then
      echo "$name: up (pid $(cat "$pidfile"))"
    else
      echo "$name: down"
    fi
  done
  echo
  for url in \
    "http://127.0.0.1:${BFF_PORT#:}/agent/healthz" \
    "http://127.0.0.1:${BFF_PORT#:}/"; do
    code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 2 "$url" || echo "ERR")
    echo "$url → $code"
  done
}

case "$cmd" in
  start)  start ;;
  stop)   stop ;;
  status) status ;;
  *) echo "uso: $0 {start|stop|status}"; exit 2 ;;
esac
