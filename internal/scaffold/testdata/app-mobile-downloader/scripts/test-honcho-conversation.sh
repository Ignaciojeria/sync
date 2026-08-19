#!/usr/bin/env bash
# scripts/test-honcho-conversation.sh
#
# Smoke E2E: crea una sesión, manda N turnos al agent con
# distintos tipos de mensajes, y al final verifica el estado en
# Honcho:
#   - peers correctos (user hash + agent ID)
#   - N mensajes por turno sin duplicación
#   - sin errores 4xx/5xx del adapter
#
# Uso: bash ./scripts/test-honcho-conversation.sh [N_TURNS]
# Default N_TURNS=8.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

N_TURNS="${1:-8}"
WS="$(grep '^HONCHO_WORKSPACE_ID=' .env | cut -d= -f2)"
KEY="$(grep '^HONCHO_API_KEY=' .env | cut -d= -f2)"
API="http://127.0.0.1:8001"

# Mensajes variados para que la conversación tenga forma.
# Cada uno referencia algo del anterior para simular follow-ups
# naturales, y un par cambian de tema para forzar al recall a
# usar search_query semántico.
MESSAGES=(
  "Hola, soy un dev backend Go. ¿Podés ayudarme a refactorizar un servicio?"
  "El servicio lee configs de un archivo JSON y las expone vía HTTP. ¿Qué patrón recomendás?"
  "Ahora imaginá que tenemos 50 servicios así. ¿Cómo lo harías escalable?"
  "Cambio de tema: ¿qué opinás de TypeScript para backend?"
  "Y comparando con Go, ¿cuál es mejor para APIs de baja latencia?"
  "Ok volviendo al refactor: ¿usarías un loader con hot-reload?"
  "Perfecto. Ahora mostrame un ejemplo mínimo en Go del loader con watch."
  "Última pregunta: ¿cómo testeo ese loader sin flaky tests?"
)

echo "== setup =="
echo "workspace: $WS"
echo "turnos: $N_TURNS"
echo

echo "== creando sesión =="
SESSION=$(curl -s -X POST \
  -H "X-Dev-Sub: dev-user" \
  -H "X-Dev-Email: dev@example.com" \
  -H "Content-Type: application/json" \
  -d '{"title":"honcho long conversation","cwd":"/home/exedev/workspace/gitinittest5","model":"claude-sonnet-4"}' \
  "$API/agent/sessions" \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['session']['id'])")
echo "session: $SESSION"
echo

# Track inicio de logs del server para el diff de warnings
LOG_OFFSET=$(wc -l < tmp/run/api.log)
echo "log offset inicial: $LOG_OFFSET líneas"
echo

START=$(date +%s)
for i in $(seq 0 $((N_TURNS - 1))); do
  MSG="${MESSAGES[$i]}"
  TURN_ID="turn-$((i + 1))"
  echo "--- turno $((i + 1))/$N_TURNS (turn_id=$TURN_ID) ---"
  echo "  msg: ${MSG:0:60}..."

  T0=$(date +%s)
  HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST \
    -H "X-Dev-Sub: dev-user" \
    -H "X-Dev-Email: dev@example.com" \
    -H "Content-Type: application/json" \
    -d "$(python3 -c "import json,sys; print(json.dumps({'message': sys.argv[1], 'turn_id': sys.argv[2]}))" "$MSG" "$TURN_ID")" \
    "$API/agent/sessions/${SESSION}/prompt")
  T1=$(date +%s)
  echo "  prompt: HTTP $HTTP_CODE en $((T1 - T0))s"

  # Esperar a que el agent termine ese turno antes de mandar el
  # siguiente. 12s es generoso para Sonnet en una respuesta
  # corta/media; subimos si vemos timeouts.
  sleep 12
done
END=$(date +%s)
echo
echo "== timing total: $((END - START))s =="

# Esperar un toque más para que el último flush de Honcho termine
sleep 3

echo
echo "== estado en Honcho =="
HONCHO_BASE="https://api.honcho.dev/v3/workspaces/${WS}/sessions/${SESSION}"

echo "--- peers ---"
curl -s -H "Authorization: Bearer ${KEY}" "$HONCHO_BASE/peers" \
  | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'total peers: {d[\"total\"]}'); [print(f'  - {p[\"id\"]}') for p in d['items']]"

echo
echo "--- mensajes (esperando que load=total) ---"
curl -s -X POST -H "Authorization: Bearer ${KEY}" \
  -H "Content-Type: application/json" \
  -d '{"limit":100}' \
  "$HONCHO_BASE/messages/list" \
  | python3 -c "
import sys, json
d = json.load(sys.stdin)
items = d['items']
print(f'total mensajes: {len(items)}')
print()
print('=== por peer ===')
from collections import Counter
by_peer = Counter(m['peer_id'] for m in items)
for peer, count in by_peer.most_common():
    print(f'  {peer}: {count} mensajes')
print()
print('=== detalle ===')
for m in items:
    kind = 'user' if m['peer_id'].startswith('user-') else 'agent'
    print(f'  [{kind}] {m[\"content\"][:70]!r}')
print()
print('=== dedup check ===')
seen = set()
dups = []
for m in items:
    key = (m['peer_id'], m['content'])
    if key in seen:
        dups.append(m['id'])
    seen.add(key)
if dups:
    print(f'  ❌ {len(dups)} duplicados encontrados')
    for did in dups[:5]:
        print(f'    - id={did}')
else:
    print(f'  ✅ sin duplicados')
print()
print('=== esperado por turno ===')
print(f'  turnos: {int(\"$N_TURNS\")}')
print(f'  mínimo esperado: {int(\"$N_TURNS\")} user + {int(\"$N_TURNS\")} agent = {int(\"$N_TURNS\") * 2}')
"

echo
echo "== logs del server (warnings/errors desde este test) =="
tail -n +"$LOG_OFFSET" tmp/run/api.log \
  | grep -E "WARN|ERROR" \
  | grep -vE "preview|preview ready|workspace cleanup" \
  | head -20 \
  | sed 's/^/  /'

echo
echo "== conclusión =="
HONCHO_TOTAL=$(curl -s -X POST -H "Authorization: Bearer ${KEY}" \
  -H "Content-Type: application/json" \
  -d '{"limit":100}' \
  "$HONCHO_BASE/messages/list" \
  | python3 -c "import sys,json; print(len(json.load(sys.stdin)['items']))")
echo "session $SESSION: $HONCHO_TOTAL mensajes en Honcho"
