// state.js — state machine mínima del cliente V2.
//
// El cliente NO deduce fases. Solo consume el `phase` que emite el
// server en el envelope SSE y refleja eso en el DOM. Esto evita
// repetir la deuda de V1 (heurísticas, timers mágicos, race
// conditions entre estados del cliente y del server).
//
// El estado vive en un objeto plano exportado. main.js lo lee y
// escribe. sse.js y composer.js también. No usamos proxies ni
// observables: el state es chico y los updates son atómicos desde
// el punto de vista del event loop.

// ponytail: las fases que el server puede emitir hoy (ver
// internal/agent/application/render.go RenderFragment).
// "queued" NO es una fase del server: la seteamos del lado del
// cliente cuando hay mensajes en la cola de pi (queue_update)
// y NO hay un turno activo. Aparece en el badge primario
// como "En cola" (variante de "Trabajando" pero sin spinner).
// "answering" sigue siendo client-side: el server lo deduce
// cuando llegan text_deltas (no emite phase para ellos), pero
// en la práctica el badge primario no distingue answering de
// thinking/tooling/etc. — todos son "Trabajando".
export const PHASES = Object.freeze({
  IDLE: "idle",
  THINKING: "thinking",
  TOOLING: "tooling",
  ANSWERING: "answering",
  COMPACTING: "compacting",
  RETRYING: "retrying",
  QUEUED: "queued",
  ERROR: "error",
});

// isWorkingPhase retorna true si el phase representa actividad
// del agente (lo que el badge primario muestra como "Trabajando"
// + spinner). Incluye queued porque también es "actividad
// pendiente" — el agent ya terminó su turno pero hay mensajes
// esperando para el próximo.
//
// ponytail: NO incluye ERROR. Error es un estado terminal
// visible por sí solo (badge rojo "Error"). Mezclar error
// con working haría que el badge cambiara de "Trabajando" a
// "Error" cuando llegue un runtime_error, lo cual técnicamente
// no es flicker pero sí es un cambio visual fuerte. Mejor
// tratarlos como categorías separadas.
export function isWorkingPhase(phase) {
  return phase === PHASES.THINKING ||
    phase === PHASES.TOOLING ||
    phase === PHASES.ANSWERING ||
    phase === PHASES.COMPACTING ||
    phase === PHASES.RETRYING ||
    phase === PHASES.QUEUED;
}

export function createState(initial = {}) {
  return {
    sessionId: "",
    phase: PHASES.IDLE,
    connected: false,
    connecting: false,
    lastUserPrompt: "",
    lastEventId: 0,
    pendingPrompt: false,
    queuedPromptText: "",
    // ponytail: queueCount lo empuja el server vía envelopes
    // kind="queue" (RenderFragment queueUpdateFragment). El
    // badge secundario "En cola: N" lo lee de acá. Se mantiene
    // aunque turn-end llegue: si quedó cola pendiente, el
    // primario sigue mostrando "En cola" en lugar de vacío.
    queueSteering: 0,
    queueFollowUp: 0,
    ...initial,
  };
}

// setPhase actualiza la fase y emite un evento v2:phase. Los
// listeners (badge, banner, etc.) se suscriben en main.js.
//
// ponytail: setPhase es NO-OP si la fase no cambió (early
// return). Esto evita re-paints innecesarios del DOM cuando
// el server re-emite el mismo phase (e.g. dos message_start
// consecutivos por algún edge case). El listener sólo corre
// cuando hay una transición real.
export function setPhase(state, phase, listeners) {
  if (state.phase === phase) return;
  state.phase = phase;
  for (const listener of listeners) {
    try {
      listener(state);
    } catch (err) {
      console.warn("[v2] phase listener error", err);
    }
  }
}

// setConnected actualiza el flag connected y notifica. La UI usa
// esto para mostrar el dot live/reconectando en el header.
//
// ponytail: el badge primario NO debe responder a connected.
// Si lo hiciera, una reconexión SSE rápida reescribiría el
// textContent aunque visualmente no cambie — eso causa
// re-paints del header que el usuario percibe como parpadeo.
// Por eso el listener de connected ya no toca el badge; sólo
// maneja el dot live/reconectando. El badge primario es
// estable durante todo un turno activo sin importar el
// estado del SSE.
export function setConnected(state, value, listeners) {
  if (state.connected === value) return;
  state.connected = value;
  for (const listener of listeners) {
    try {
      listener(state);
    } catch (err) {
      console.warn("[v2] connected listener error", err);
    }
  }
}
