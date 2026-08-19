#!/usr/bin/env bash
# scripts/test-honcho-recall.sh
#
# E2E test del recall cross-session:
#   1. Crea sesión 1 con 6 turnos sobre un tema (Go vs TS).
#   2. Espera un poco para que Honcho razone.
#   3. Crea sesión 2 con el mismo user, fresh.
#   4. Manda UN prompt sobre el mismo tema.
#   5. Verifica que el recall (inyectado como steer) trae
#      contexto de la sesión 1.
#
# Limitación: Honcho background reasoning tarda minutos. Si
# los messages todavía no se procesaron, el recall devuelve
# string vacío (no es bug, es timing).
#
# Uso: bash ./scripts/test-honcho-recall.sh

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

WS="$(grep '^HONCHO_WORKSPACE_ID=' .env | cut -d= -f2)"
KEY="$(grep '^HONCHO_API_KEY=' .env | cut -d= -f2)"
API="http://127.0.0.1:8001"

# Mensajes de la sesión 1 — sobre el mismo tema, con detalle
# que debería aparecer en la representación del peer.
TOPIC_MESSAGES=(
  "Trabajo en un servicio Go de baja latencia (~5ms p99). Tengo 12 servicios así."
  "El cuello de botella es GC. Go tiene pausas de 1-2ms en collections grandes. ¿Es normal?"
  "Probé GOGC=200 y bajó la frecuencia de collections pero las pausas individuales son más largas."
  "Para nuestro caso, latency-critical, ¿convendría Rust o seguir con Go tuning?"
  "Tenés razón. Nos conviene Go con sync.Pool y object pooling agresivo antes que cambiar de lenguaje."
  "Voy a empezar midiendo alloc/op con pprof en el hot path. Gracias."
)

# Prompt de la sesión 2 — referencia implícita al tema.
CROSS_SESSION_PROMPT="¿Me recomendás un patrón para reducir allocs en servicios Go de alta concurrencia? Tengo un cuello de botella que ya medí con pprof pero no sé qué mirar primero."

echo "== fase 1: sesión 1 (construir memoria) =="
SESSION1=$(curl -s -X POST \
  -H "X-Dev-Sub: dev-user" -H "X-Dev-Email: dev@example.com" \
  -H "Content-Type: application/json" \
  -d '{"title":"recall test sesión 1","cwd":"/home/exedev/workspace/gitinittest5","model":"claude-sonnet-4"}' \
  "$API/agent/sessions" \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['session']['id'])")
echo "sesión 1: $SESSION1"

for i in "${!TOPIC_MESSAGES[@]}"; do
  MSG="${TOPIC_MESSAGES[$i]}"
  TURN_ID="s1-turn-$((i + 1))"
  echo "  s1 turno $((i + 1))/${#TOPIC_MESSAGES[@]}: ${MSG:0:50}..."
  curl -s -o /dev/null -X POST \
    -H "X-Dev-Sub: dev-user" -H "X-Dev-Email: dev@example.com" \
    -H "Content-Type: application/json" \
    -d "$(python3 -c "import json,sys; print(json.dumps({'message': sys.argv[1], 'turn_id': sys.argv[2]}))" "$MSG" "$TURN_ID")" \
    "$API/agent/sessions/${SESSION1}/prompt"
  sleep 12
done

echo
echo "== estado de Honcho para sesión 1 =="
HONCHO_BASE_S1="https://api.honcho.dev/v3/workspaces/${WS}/sessions/${SESSION1}"
curl -s -X POST -H "Authorization: Bearer ${KEY}" \
  -H "Content-Type: application/json" \
  -d '{"limit":50}' \
  "$HONCHO_BASE_S1/messages/list" \
  | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'  mensajes: {len(d[\"items\"])}')"

echo
echo "== fase 2: esperando razonamiento de Honcho =="
echo "Honcho tarda minutos en extraer conclusions. Esperando 240s..."
sleep 240

echo
echo "== fase 3: sesión 2 (mismo user, fresh) =="
SESSION2=$(curl -s -X POST \
  -H "X-Dev-Sub: dev-user" -H "X-Dev-Email: dev@example.com" \
  -H "Content-Type: application/json" \
  -d '{"title":"recall test sesión 2","cwd":"/home/exedev/workspace/gitinittest5","model":"claude-sonnet-4"}' \
  "$API/agent/sessions" \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['session']['id'])")
echo "sesión 2: $SESSION2"

echo "  prompt cross-session: $CROSS_SESSION_PROMPT"
curl -s -o /dev/null -X POST \
  -H "X-Dev-Sub: dev-user" -H "X-Dev-Email: dev@example.com" \
  -H "Content-Type: application/json" \
  -d "$(python3 -c "import json,sys; print(json.dumps({'message': sys.argv[1], 'turn_id': 's2-turn-1'}))" "$CROSS_SESSION_PROMPT")" \
  "$API/agent/sessions/${SESSION2}/prompt"
sleep 12

echo
echo "== estado de Honcho para sesión 2 =="
HONCHO_BASE_S2="https://api.honcho.dev/v3/workspaces/${WS}/sessions/${SESSION2}"
curl -s -X POST -H "Authorization: Bearer ${KEY}" \
  -H "Content-Type: application/json" \
  -d '{"limit":50}' \
  "$HONCHO_BASE_S2/messages/list" \
  | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'  mensajes: {len(d[\"items\"])}')"

echo
echo "== recall (lo que se inyectó en el prompt de s2) =="
echo "(si Honcho ya razonó, debería traer algo de s1. Si no, string vacío.)"
echo
curl -s -H "Authorization: Bearer ${KEY}" \
  "$HONCHO_BASE_S2/context?tokens=1000&search_query=reducir+allocs+en+servicios+Go&peer_target=agent-${SESSION2}&summary=true" \
  | python3 -m json.tool 2>&1 | head -50
