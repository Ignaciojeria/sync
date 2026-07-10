#!/usr/bin/env bash
# scripts/run-api.sh — modo dev simple: una sola app (cmd/api).
#
# Uso:
#   bash ./scripts/run-api.sh start
#   bash ./scripts/run-api.sh stop
#   bash ./scripts/run-api.sh status
#
# Nota: .air.toml ya compila ./cmd/api -> ./tmp/main. Este script es
# útil cuando querés correr sin air o reiniciar limpio en la VM.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PIDDIR="$ROOT/tmp/run"
mkdir -p "$PIDDIR"

PORT="${PORT:-8001}"
cmd="${1:-start}"

build_binary() {
  echo "+ go build -o $ROOT/tmp/main ./cmd/api"
  ( cd "$ROOT" && go build -o ./tmp/main ./cmd/api )
}

start() {
  build_binary

  pidfile="$PIDDIR/api.pid"
  if [ -f "$pidfile" ] && kill -0 "$(cat "$pidfile")" 2>/dev/null; then
    echo "api ya está corriendo (pid $(cat "$pidfile"))"
    return
  fi

  if ss -ltn 2>/dev/null | grep -qE ":${PORT}[[:space:]]"; then
    holder=$(ss -ltnp 2>/dev/null | grep -E ":${PORT} " | grep -oE 'pid=[0-9]+' | head -1 || true)
    echo "❌  :${PORT} ya está en uso ${holder:+($holder)}." >&2
    exit 1
  fi

  echo "+ start api: env PORT=$PORT $ROOT/tmp/main"
  export LOG_OUT="$PIDDIR/api.log"
  bash -c 'exec "$@">"$LOG_OUT" 2>&1' -- env PORT="$PORT" "$ROOT/tmp/main" </dev/null &
  echo $! > "$pidfile"
  unset LOG_OUT

  echo
  echo "App única:"
  echo "  browser ─► web-server :$PORT"
  echo
  echo "Probad:"
  echo "  curl -i http://127.0.0.1:$PORT/"
  echo "  curl -i http://127.0.0.1:$PORT/agent"
}

stop() {
  pidfile="$PIDDIR/api.pid"
  if [ -f "$pidfile" ]; then
    pid="$(cat "$pidfile")"
    if kill -0 "$pid" 2>/dev/null; then
      echo "+ stop api (pid $pid)"
      kill "$pid" || true
    fi
    rm -f "$pidfile"
  fi
}

status() {
  pidfile="$PIDDIR/api.pid"
  if [ -f "$pidfile" ] && kill -0 "$(cat "$pidfile")" 2>/dev/null; then
    echo "api: up (pid $(cat "$pidfile"))"
  else
    echo "api: down"
  fi
  echo
  for url in \
    "http://127.0.0.1:${PORT}/" \
    "http://127.0.0.1:${PORT}/agent"; do
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
