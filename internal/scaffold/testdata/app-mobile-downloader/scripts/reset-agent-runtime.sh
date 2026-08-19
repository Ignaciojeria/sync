#!/usr/bin/env bash
set -euo pipefail

# ponytail: borra solo estado runtime/temporal del agente. No toca código fuente.
# Uso:
#   bash ./scripts/reset-agent-runtime.sh
# Luego reiniciar el server/app.

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
REPO_NAME="$(basename "$ROOT")"
PARENT_DIR="$(dirname "$ROOT")"
WORKTREES_DIR="$PARENT_DIR/.${REPO_NAME}-agent-workspaces"

SESSIONS_DIR="${AGENT_SESSION_DIR:-$ROOT/tmp/agent-sessions}"
PI_SESSIONS_DIR="$ROOT/tmp/agent-pi-sessions"
EVENTS_DIR="$ROOT/tmp/agent-events"
SANDBOX_DIR="$ROOT/tmp/agent-work"

printf 'reset-agent-runtime:\n'
printf '  root: %s\n' "$ROOT"
printf '  sessions: %s\n' "$SESSIONS_DIR"
printf '  pi sessions: %s\n' "$PI_SESSIONS_DIR"
printf '  events: %s\n' "$EVENTS_DIR"
printf '  sandbox: %s\n' "$SANDBOX_DIR"
printf '  worktrees: %s\n' "$WORKTREES_DIR"

rm -rf "$SESSIONS_DIR" \
       "$PI_SESSIONS_DIR" \
       "$EVENTS_DIR" \
       "$SANDBOX_DIR" \
       "$WORKTREES_DIR"

mkdir -p "$SESSIONS_DIR" "$PI_SESSIONS_DIR" "$EVENTS_DIR"

printf '\nOK: runtime state cleared. Restart the app/server and create a new session.\n'
