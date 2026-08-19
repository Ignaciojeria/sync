// sse.js — conexión SSE + reconnect + Last-Event-ID.
//
// El cliente V2 NO decide cuándo mostrar "thinking" vs "answering":
// eso viene en el envelope `phase` del server. Sólo consume y
// refleja. Si el server dice "thinking", mostramos "thinking".
// Cuando dice "idle", limpiamos el banner.
//
// Reconexión con backoff exponencial (1s → 30s) + jitter. Si el
// cliente tenía un lastEventId, lo manda como query param ?resume=N
// para que el server reenvíe los eventos perdidos mientras
// estuvimos offline.
//
// El módulo expone `keepAlive(state, handlers)` que envuelve connect
// con backoff. Devuelve una promesa que sólo resuelve si el caller
// aborta la sesión (state.sessionId cambia a "").

import { PHASES, setConnected } from "./state.js";
const MAX_BACKOFF_MS = 30000;
const BASE_BACKOFF_MS = 1000;

// handlers es el shape que keepAlive() espera:
//   onFragment({ id, html, itemKind }) → upsert en el feed
//   onPhase(phase) → refleja el phase badge
//   onQueue({ steering, followUp }) → badge secundario "En cola: N"
//   onTurnEnd() → limpia banner + scrolls
//   onConnected(boolean) → dot live/reconectando
//   onError(message) → muestra error visible
export async function keepAlive(state, handlers) {
  let attempts = 0;
  while (state.sessionId) {
    await connect(state, handlers);
    if (!state.sessionId) return;
    attempts++;
    setConnected(state, false, [handlers.onConnectedChange]);
    const delay = backoffDelay(attempts);
    await sleep(delay);
    if (!state.sessionId) return;
    attempts = 0;
  }
}

async function connect(state, handlers) {
  if (!state.sessionId) return;

  const controller = new AbortController();
  const lastId = state.lastEventId || "";

  let url = `/agent/sessions/${encodeURIComponent(state.sessionId)}/events`;
  if (lastId) {
    url += `?resume=${encodeURIComponent(String(lastId))}`;
  } else {
    url += `?resume=live`;
  }

  try {
    const response = await fetch(url, {
      headers: {
        Accept: "text/event-stream",
        ...(lastId ? { "Last-Event-ID": String(lastId) } : {}),
      },
      signal: controller.signal,
    });
    if (!response.ok || !response.body) {
      handlers.onError?.(`SSE: HTTP ${response.status}`);
      return;
    }

    setConnected(state, true, [handlers.onConnectedChange]);
    await readStream(response, state, controller, handlers);
  } catch (err) {
    if (err?.name !== "AbortError") {
      console.warn("[v2] sse error", err);
    }
  }
}

async function readStream(response, state, controller, handlers) {
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split("\n");
      buffer = lines.pop();

      let currentEvent = "message";
      let currentId = "";
      for (const line of lines) {
        if (!line) continue;
        if (line.startsWith(":")) continue;
        if (line.startsWith("event:")) {
          currentEvent = line.slice(6).trim();
          continue;
        }
        if (line.startsWith("id:")) {
          const v = line.slice(3).trim();
          if (v !== "" && !Number.isNaN(Number(v))) {
            currentId = v;
            state.lastEventId = Number(v);
          }
          continue;
        }
        if (line.startsWith("data:")) {
          const data = line.slice(5).trim();
          handlePayload(currentEvent, data, handlers);
        }
      }
    }
  } catch (err) {
    if (err?.name !== "AbortError") {
      console.warn("[v2] stream read error", err);
    }
    throw err;
  }
}

function handlePayload(eventName, rawData, handlers) {
  if (eventName !== "agent-fragment") return;
  let envelope;
  try {
    envelope = JSON.parse(rawData);
  } catch (err) {
    console.warn("[v2] payload no es JSON", err, rawData);
    return;
  }
  switch (envelope.kind) {
    case "fragment":
      handlers.onFragment?.(envelope);
      return;
    case "phase":
      handlers.onPhase?.(safePhase(envelope.phase));
      return;
    // ponytail: el server emite DOS tipos de cierre de turno.
    //   - "synthetic-end": llega después de CADA message_end (pi
    //     emite uno por chunk de streaming). Sólo destraba el
    //     composer; NO toca el phase. Si tocara el phase, el
    //     cliente haría flicker entre working/idle 4-5 veces por
    //     segundo durante un turno largo.
    //   - "turn-end": el real (agent_end, turn_end, runtime_exit).
    //     Limpia el phase a idle y vacía el badge.
    case "synthetic-end":
      handlers.onSyntheticEnd?.();
      return;
    case "turn-end":
      handlers.onTurnEnd?.();
      return;
    // ponytail: el server emite kind="queue" cuando pi manda
    // queue_update con cambios en la cola steering/follow-up.
    // Es metadata aditiva: NO toca el phase primario. Sólo
    // refresca el badge secundario "En cola: N" y, si el
    // turno actual ya cerró y quedó cola pendiente, el cliente
    // puede subir el primario a "queued" para evitar el
    // flicker del gap entre turnos.
    case "queue":
      handlers.onQueue?.({
        steering: envelope.queueSteering || 0,
        followUp: envelope.queueFollowUp || 0,
      });
      return;
    default:
      return;
  }
}

function backoffDelay(attempt) {
  const base = Math.min(
    MAX_BACKOFF_MS,
    BASE_BACKOFF_MS * Math.pow(2, Math.max(0, attempt - 1))
  );
  const jitter = base * 0.2 * (Math.random() * 2 - 1);
  return Math.max(500, Math.round(base + jitter));
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

export function safePhase(value) {
  if (Object.values(PHASES).includes(value)) return value;
  return PHASES.IDLE;
}
