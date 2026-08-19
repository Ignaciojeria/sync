// main.js — entrypoint del módulo V2.
//
// Bootstrap:
//   1. Lee data-* del root (#v2-app) para conocer sessionId, JWT, etc.
//   2. Crea el state machine local.
//   3. Wirea el composer (input + submit).
//   4. Wirea la sidebar (delete).
//   5. Conecta el SSE y arranca el loop de keepAlive.
//
// El cliente NO deduce fases: cuando sse.js recibe un `phase`, lo
// refleja en el badge del header. Cuando recibe un fragment, lo
// upserta en el feed. Cuando recibe turn-end, limpia el banner.

import { createState, setPhase, PHASES, isWorkingPhase } from "./state.js";
import { keepAlive, safePhase } from "./sse.js";
import { createFeed, wireCopyButtons, wireRegenerateButtons } from "./feed.js";
import { createComposer } from "./composer.js";
import { wireSidebar, deleteSession, createSession, refreshSidebarPreview } from "./sidebar.js";
import { createBudget } from "./budget.js";
import { createWorktreePanel } from "./worktree-panel.js";
import { applyServerHtml, forceScrollToBottom } from "./dom.js";

const root = document.getElementById("v2-app");
if (!root) {
  console.warn("[v2] root #v2-app not found, aborting bootstrap");
} else {
  bootstrap(root);
}

function bootstrap(rootEl) {
  const sessionId = rootEl.dataset.activeSessionId || "";
  const state = createState({ sessionId });

  // --- DOM refs ---------------------------------------------------------
  // ponytail: el feed tiene DOS refs distintas porque el
  // contenedor outer (#v2-feed-wrap) tiene overflow-y: auto y es
  // el que scrollea, mientras que el inner (#v2-feed) es donde
  // appendeamos/upsertamos items. Confundirlos rompe el scroll.
  // Ver createFeed en feed.js.
  const feedWrapEl = rootEl.querySelector("#v2-feed-wrap");
  const feedEl = rootEl.querySelector("#v2-feed");
  const emptyEl = rootEl.querySelector(".v2-empty");
  const phaseBadge = rootEl.querySelector("#v2-phase-badge");
  const phaseBadgeText = phaseBadge?.querySelector(".v2-phase-badge-text");
  const phaseDetailBadge = rootEl.querySelector("#v2-phase-detail-badge");
  const phaseDetailToggle = rootEl.querySelector("#v2-phase-detail-toggle");
  const abortButton = rootEl.querySelector("#v2-abort");
  const formEl = rootEl.querySelector("#v2-composer");
  const inputEl = rootEl.querySelector("#v2-input");
  const sendButton = rootEl.querySelector("#v2-send");
  const sidebarEl = rootEl.querySelector("#v2-sessions-list");

  const feed = createFeed(feedWrapEl, feedEl, emptyEl);

  // ponytail: M-C.1 = wireCopyButtons() activa el handler
  // delegado para los botones Copy de cada item. El handler
  // lee textContent del item y copia al clipboard. Se llama
  // una sola vez al boot — el handler se queda escuchando
  // clicks via event delegation sobre el feed.
  wireCopyButtons(feedEl);

  // ponytail: M-C.2 = wireRegenerateButtons wirea los botones
  // de Regenerate. Cuando el user click → POST al endpoint →
  // borra los items viejos del feed (con seq > clearAfterSeq)
  // → los nuevos fragments del assistant llegan por SSE y se
  // renderizan normalmente.
  wireRegenerateButtons(feedEl, state.sessionId, (clearAfterSeq) => {
    feed.clearFromSeq(clearAfterSeq);
    // ponytail: resetea el botón Regenerate que esté en estado
    // "regenerating" — el cliente ya borró los items viejos del
    // feed, así que los botones que apuntaban a esos items ya
    // no existen. Si hay un botón en estado regenerating (de un
    // item que ya no está), lo limpiamos por las dudas.
    const regenBtns = feedEl.querySelectorAll('[data-regenerate-button][data-state="regenerating"]');
    for (const b of regenBtns) {
      delete b.dataset.state;
      b.setAttribute("aria-label", "Regenerar respuesta");
    }
  });

  // --- Phase badge sync -------------------------------------------------
  // ponytail: el badge primario muestra UN SOLO estado: "está
  // trabajando o no". No distingue thinking vs tooling vs
  // answering vs compacting — eso es ruido de implementación
  // interna que al usuario no le importa. Pi en la terminal hace
  // lo mismo: muestra "Working" continuo durante todo el turno,
  // sin importar la sub-fase. Aquí también: el badge aparece con
  // el primer phase activo y se va con el primer phase idle.
  //
  // Filtros anti-flicker:
  //   1. Si textContent no cambia respecto al último render, NO
  //      tocamos el DOM. Antes reescribíamos textContent en cada
  //      setPhase aunque el texto fuera idéntico — eso causaba
  //      re-paints visibles del header.
  //   2. Si dataset.state no cambia, tampoco tocamos el atributo.
  //   3. El estado "queued" tiene texto distinto ("En cola") para
  //      que el usuario sepa que hay mensajes pendientes aunque
  //      no haya turno activo. Sigue siendo "trabajo pendiente"
  //      semánticamente, pero el texto cambia para no confundir.
  let lastPrimaryLabel = null;
  let lastPrimaryState = null;
  function primaryState(phase) {
    if (isWorkingPhase(phase) || phase === PHASES.QUEUED) return "working";
    if (phase === PHASES.ERROR) return "error";
    return "idle";
  }
  // ponytail: el badge ahora vive en el budget bar (al lado
  // del balance), NO en el header. El DOM es:
  //   <span id="v2-phase-badge">
  //     <span class="v2-phase-badge-text"></span>
  //     <span class="v2-phase-badge-dots">
  //       <span class="v2-working-dot"/>
  //       <span class="v2-working-dot"/>
  //       <span class="v2-working-dot"/>
  //     </span>
  //   </span>
  // El texto lo escribimos en .v2-phase-badge-text. Los dots
  // son CSS puro — su visibilidad depende del data-state del
  // badge raíz (data-state="working" los muestra, otros los
  // ocultan). El JS sólo cambia el texto y el data-state,
  // nunca toca los dots directamente. Eso evita re-paints
  // innecesarios durante un turno activo.
  //
  function primaryLabel(phase) {
    if (phase === PHASES.QUEUED) return "En cola";
    if (phase === PHASES.ERROR) return "Error";
    if (isWorkingPhase(phase)) return "Generando…";
    return "";
  }
  function reflectPhase() {
    if (!phaseBadge) return;
    const label = primaryLabel(state.phase);
    const dataState = primaryState(state.phase);
    // ponytail: doble check de idempotencia. Si tanto el texto
    // como el data-state no cambiaron, retornamos sin tocar el
    // DOM. Esto es CRÍTICO para evitar re-paints del budget
    // bar — el badge ahora vive ahí y cualquier repaint es
    // visible porque está cerca del balance.
    if (label === lastPrimaryLabel && dataState === lastPrimaryState) return;
    lastPrimaryLabel = label;
    lastPrimaryState = dataState;
    if (phaseBadgeText) phaseBadgeText.textContent = label;
    phaseBadge.dataset.state = dataState;
  }
  // ponytail: el connection state ya NO toca el badge primario.
  // Si lo hiciera, una reconexión SSE rápida reescribiría el
  // textContent aunque visualmente no cambie — eso causa
  // re-paints del header que el usuario percibe como parpadeo.
  // El dot live/reconectando se maneja por separado en otro
  // listener (no implementado todavía en este fix; ver
  // v2LiveDot si lo necesitamos).
  function reflectConnected() {
    /* no-op intencional: el badge primario es estable durante
     * todo un turno activo sin importar el estado del SSE. */
  }

  // --- Phase detail badge (secundario, opt-in) --------------------------
  // ponytail: el badge secundario muestra la sub-fase real
  // (Pensando / Tool / Respondiendo / Compactando / Reintentando)
  // sólo cuando el usuario lo activa con el toggle. Por defecto
  // está OCULTO para no agregar ruido a la UI — la mayoría de los
  // turnos no necesitan ese nivel de detalle.
  //
  // El secundario se actualiza con cada phase sin tocar el
  // primario. Si llegan 5 text_deltas consecutivos, el secundario
  // no cambia (sigue en "Respondiendo") y por tanto no causa
  // re-paints. Sólo cambia cuando hay una transición real
  // (thinking → tooling → answering).
  const phaseDetailLabels = {
    [PHASES.IDLE]: "",
    [PHASES.THINKING]: "Pensando",
    [PHASES.TOOLING]: "Ejecutando tool",
    [PHASES.ANSWERING]: "Respondiendo",
    [PHASES.COMPACTING]: "Compactando",
    [PHASES.RETRYING]: "Reintentando",
    [PHASES.QUEUED]: "En cola",
    [PHASES.ERROR]: "Error",
  };
  // ponytail: el secundario lo actualiza un único punto
  // (renderSecondary). Cualquier caller (setPhase listeners,
  // onQueue, applyDetailVisibility) llama a renderSecondary y
  // este decide qué pintar. Antes había dos callers tocando
  // lastDetailLabel con significados distintos (renderSecondary
  // vs onQueue), lo que causaba que el secundario quedara en ""
  // durante un turno activo si llegaba un queue_update con
  // total=0. El estado del badge lo deriva SIEMPRE de state.phase
  // + queueCount; nunca se pinta texto ad-hoc desde otros
  // handlers.
  let lastSecondaryText = null;
  let lastSecondarySubphase = null;
  function renderSecondary() {
    if (!phaseDetailBadge || !detailEnabled) return;
    const total = (state.queueSteering || 0) + (state.queueFollowUp || 0);
    // ponytail: la cola tiene prioridad sobre el phase. Si
    // hay mensajes encolados (steering/followUp), el badge
    // muestra "En cola: N" aunque el turno actual siga
    // activo. Esto evita que el usuario pierda de vista la
    // cola mientras mira el "Pensando" actual.
    let text = "";
    let subphase = "idle";
    if (total > 0) {
      text = `En cola: ${total}`;
      subphase = "queued";
    } else if (isWorkingPhase(state.phase)) {
      text = phaseDetailLabels[state.phase] || "";
      subphase = state.phase;
    } else if (state.phase === PHASES.ERROR) {
      text = phaseDetailLabels[PHASES.ERROR] || "Error";
      subphase = "error";
    }
    if (text === lastSecondaryText && subphase === lastSecondarySubphase) return;
    lastSecondaryText = text;
    lastSecondarySubphase = subphase;
    phaseDetailBadge.textContent = text;
    phaseDetailBadge.dataset.subphase = subphase;
  }

  // ponytail: el badge secundario empieza OCULTO. El usuario lo
  // activa con el toggle al lado del primario. Persistimos la
  // preferencia en sessionStorage (no localStorage) para que la
  // próxima pestaña también la respete pero no quede flotando
  // entre sesiones distintas del browser.
  const DETAIL_PREF_KEY = "agentV2.phaseDetail";
  let detailEnabled = (() => {
    try {
      return sessionStorage.getItem(DETAIL_PREF_KEY) === "1";
    } catch (_) {
      return false;
    }
  })();
  function applyDetailVisibility() {
    if (!phaseDetailBadge) return;
    phaseDetailBadge.hidden = !detailEnabled;
    if (phaseDetailToggle) {
      phaseDetailToggle.setAttribute("aria-pressed", detailEnabled ? "true" : "false");
      phaseDetailToggle.dataset.enabled = detailEnabled ? "true" : "false";
    }
    if (detailEnabled) {
      // ponytail: forzar re-render completo cuando el toggle
      // se enciende. Limpiamos lastSecondaryText para que
      // renderSecondary pinte aunque el contenido sea el mismo
      // que tenía antes (que pudo haber sido escrito por
      // onQueue con la lógica vieja).
      lastSecondaryText = null;
      lastSecondarySubphase = null;
      renderSecondary();
    } else {
      phaseDetailBadge.textContent = "";
      lastSecondaryText = "";
      lastSecondarySubphase = "idle";
    }
  }
  if (phaseDetailToggle) {
    phaseDetailToggle.addEventListener("click", () => {
      detailEnabled = !detailEnabled;
      try {
        sessionStorage.setItem(DETAIL_PREF_KEY, detailEnabled ? "1" : "0");
      } catch (_) { /* sessionStorage no disponible */ }
      applyDetailVisibility();
    });
  }

  // --- Composer ---------------------------------------------------------
  const composer = createComposer({
    formEl,
    inputEl,
    sendButton,
    state,
    callbacks: {
      onSubmit: async (text) => {
        if (!state.sessionId) return;
        // ponytail: UI optimista. Mostramos el mensaje del
        // usuario ANTES de que el server confirme y subimos el
        // phase a THINKING inmediatamente, sin esperar el
        // message_start del server. Si el POST falla, dejamos
        // el mensaje visible (el server lo va a ignorar pero
        // el usuario ve lo que mandó) y revertimos el phase.
        //
        // Sin este setPhase optimista, el badge quedaba en IDLE
        // durante 1-2s entre el click del usuario y el primer
        // message_start del server — un gap visible como
        // flicker. Ahora el badge aparece "Trabajando" al
        // instante.
        feed.appendUser(text);
        composer.setLocked(true);
        setPhase(state, PHASES.THINKING, [reflectPhase, reflectAbort, renderSecondary]);
        try {
          await postPrompt(text);
        } catch (err) {
          console.warn("[v2] prompt failed; reverting optimistic phase", err);
          // Si falló el POST, revertir el phase a IDLE (sólo
          // si todavía no llegó un message_start real del
          // server — si llegó, el setPhase real ya ganó).
          if (state.phase === PHASES.THINKING) {
            setPhase(state, PHASES.IDLE, [reflectPhase, reflectAbort, renderSecondary]);
          }
        }
      },
    },
  });

  // --- Sidebar ----------------------------------------------------------
  // wireSidebar maneja dos cosas:
  //   1. click en botón delete de cada sesión
  //   2. submit del form "v2-new-session" (botón "+" en la sidebar)
  // Sin el segundo, el "+" no hace nada — bug detectado en M-A.
  wireSidebar(sidebarEl, {
    onDelete: deleteSession,
    onCreate: createSession,
  });

  // --- Mobile sidebar toggle --------------------------------------------
  // ponytail: el hamburger sólo se muestra en mobile (CSS
  // .v2-sidebar-open { display: none } por default y
  // display: inline-flex en @media (max-width: 720px)). El toggle
  // usa data-open en sidebar + backdrop. Escape cierra. Click en
  // un link de sesión también cierra (porque navega). El cliente
  // JS es el dueño del estado del drawer, no el HTML.
  const sidebarOpenBtn = rootEl.querySelector("#v2-sidebar-open");
  const sidebarElMain = rootEl.querySelector(".v2-sidebar");
  const backdrop = rootEl.querySelector("#v2-sidebar-backdrop");

  function setSidebarOpen(open) {
    if (!sidebarElMain) return;
    sidebarElMain.dataset.open = open ? "true" : "false";
    if (backdrop) backdrop.dataset.open = open ? "true" : "false";
    if (sidebarOpenBtn) {
      sidebarOpenBtn.setAttribute("aria-expanded", open ? "true" : "false");
    }
  }

  if (sidebarOpenBtn) {
    sidebarOpenBtn.addEventListener("click", () => setSidebarOpen(true));
  }
  if (backdrop) {
    backdrop.addEventListener("click", () => setSidebarOpen(false));
  }
  document.addEventListener("keydown", (e) => {
    if (e.key !== "Escape") return;
    if (sidebarElMain?.dataset.open === "true") {
      setSidebarOpen(false);
    }
  });
  // Click en un link de sesión dentro del sidebar → cerrar el drawer
  // antes de navegar. Sólo en mobile (en desktop el sidebar no es
  // drawer y no necesita este wiring, pero el listener no molesta).
  sidebarEl?.addEventListener("click", (e) => {
    const link = e.target instanceof Element ? e.target.closest("a") : null;
    if (link && sidebarElMain?.dataset.open === "true") {
      setSidebarOpen(false);
    }
  });

  // --- Abort button -----------------------------------------------------
  if (abortButton) {
    abortButton.hidden = true;
    abortButton.addEventListener("click", async () => {
      if (!state.sessionId) return;
      try {
        await fetch(`/agent/sessions/${encodeURIComponent(state.sessionId)}/abort`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: "{}",
        });
      } catch (err) {
        console.warn("[v2] abort failed", err);
      }
    });
  }

  function reflectAbort() {
    if (!abortButton) return;
    const running =
      state.phase !== PHASES.IDLE &&
      state.phase !== PHASES.ERROR;
    abortButton.hidden = !running || !state.sessionId;
  }

  // --- SSE handlers -----------------------------------------------------
  const handlers = {
    onFragment(envelope) {
      // ponytail: el primer fragment de un turno del assistant
      // abre la burbuja. El cliente no tiene que distinguir
      // entre "primer delta" y "siguiente delta" — el upsert por
      // data-msg hace el trabajo.
      feed.applyFragment(envelope);
    },
    onPhase(phase) {
      const p = safePhase(phase);
      setPhase(state, p, [reflectPhase, reflectAbort, renderSecondary]);
      // Si el server dice que el turno cerró, liberamos el
      // composer para que el usuario pueda mandar el próximo
      // mensaje sin esperar al SSE de turno cerrado (que también
      // llega, pero esto reduce el tiempo de respuesta visible).
      if (p === PHASES.IDLE || p === PHASES.ERROR) {
        composer.setLocked(false);
      } else {
        composer.setLocked(true);
      }
    },
    // ponytail: el turn-end SINTÉTICO llega después de CADA
    // message_end (pi emite uno por chunk de streaming). Sólo
    // destraba el composer porque el draft ya está cerrado. NO
    // toca el phase — eso queda hasta el turn-end real
    // (agent_end, turn_end, runtime_exit). Si el synthetic tocara
    // el phase, el cliente haría flicker entre working/idle 4-5
    // veces por segundo durante un turno largo.
    onSyntheticEnd() {
      composer.setLocked(false);
    },
    onTurnEnd() {
      // ponytail: anti-bug de "En cola" stale. Si phase era
      // working (turno activo), pasamos a IDLE y reseteamos
      // los contadores de cola. La razón:
      //
      //   1. Durante un turno activo, onQueue IGNORA los
      //      queue_updates del server (early return por
      //      isWorkingPhase), así que state.queueSteering/
      //      queueFollowUp pueden tener valores stale de turnos
      //      anteriores o del estado inicial (0).
      //   2. Si esos valores stale son > 0, el código viejo
      //      pasaba a QUEUED aunque ya no haya cola, y el
      //      badge quedaba en "En cola" indefinidamente.
      //
      // La fix: cuando el turno activo termina, NO leemos los
      // contadores de cola (son stale). Pasamos a IDLE
      // siempre. Si el server tiene cola pendiente, va a
      // emitir un queue_update o message_start casi
      // inmediatamente, que el cliente procesará. El gap
      // visual entre IDLE y el próximo phase es de ms y no
      // genera flicker perceptible.
      //
      // Si phase ya era IDLE o QUEUED, este turn_end es
      // inesperado (turn_end sin turno activo). No tocamos
      // el phase — el server es la fuente de verdad.
      if (state.phase !== PHASES.IDLE &&
          state.phase !== PHASES.ERROR &&
          state.phase !== PHASES.QUEUED) {
        state.queueSteering = 0;
        state.queueFollowUp = 0;
        renderSecondary();
        setPhase(state, PHASES.IDLE, [reflectPhase, reflectAbort, renderSecondary]);
      }
      // ponytail: refrescar el preview del último mensaje del
      // assistant en el sidebar. El server guardó el preview
      // en Session.LastPreview cuando llegó el message_end /
      // tool / error. El GET in-place actualiza el item del
      // sidebar sin recargar la lista entera.
      refreshSidebarPreview(state.sessionId);
      composer.setLocked(false);
    },
    // ponytail: el server envía envelopes kind="queue" cuando pi
    // emite queue_update. Es metadata aditiva — no toca el
    // phase primario directamente, sólo refresca el contador y,
    // si el turno actual ya cerró y hay cola, sube el primario
    // a QUEUED. Esto elimina el flicker del gap entre turnos
    // cuando el usuario encola un follow-up.
    onQueue({ steering, followUp }) {
      // ponytail: durante un turno activo, los queue_updates
      // que reporta pi son RUIDO, no señal. Cuando un mensaje
      // de la cola se entrega al agent (steering o followUp),
      // pi emite queue_update con la cola decrementada, y
      // cuando la cola queda en cero (todos los mensajes se
      // entregaron) emite queue_update con cola vacía. Esas
      // oscilaciones pasan mientras el agent está trabajando
      // y NO deben tocar el state del cliente. Si las
      // procesáramos, el secundario cambiaría de 'Pensando' a
      // 'En cola: 1' a 'Pensando' entre bloques de thinking,
      // y como el primario está al lado, el usuario lo
      // percibe como flicker del badge de la topbar.
      //
      // Sólo procesamos queue_updates cuando NO hay turno
      // activo (phase IDLE) — ahí sí importan para decidir si
      // el primario debe mostrar 'En cola' o quedarse vacío.
      if (isWorkingPhase(state.phase)) return;
      state.queueSteering = steering;
      state.queueFollowUp = followUp;
      const total = steering + followUp;
      renderSecondary();
      if (state.phase === PHASES.IDLE && total > 0) {
        setPhase(state, PHASES.QUEUED, [reflectPhase, reflectAbort, renderSecondary]);
      } else if (state.phase === PHASES.QUEUED && total === 0) {
        setPhase(state, PHASES.IDLE, [reflectPhase, reflectAbort, renderSecondary]);
      }
    },
    onConnectedChange() {
      reflectConnected();
    },
    onError(message) {
      console.warn("[v2] sse error", message);
      setPhase(state, PHASES.ERROR, [reflectPhase, reflectAbort, renderSecondary]);
    },
  };

  // --- Budget bar -------------------------------------------------------
  // ponytail: el budget bar es opcional. Si el server no inyecta
  // el snapshot (state.BalanceUSD==nil y SessionCostReady==false),
  // arranca en estado "empty" y los polls pueblan los números. Si
  // el gateway está caído, mantenemos el último valor conocido.
  const budget = createBudget({ root: rootEl, sessionId });
  budget.start();

  // ponytail: Worktree Inspector Panel. Crea el handler del
  // toggle + fetch. NO abre el panel automaticamente — el user
  // lo abre con el boton del header. Cuando se abre, hace el
  // primer fetch y luego refresca cada 5s.
  const worktreePanel = createWorktreePanel(rootEl, sessionId);

  // --- Boot -------------------------------------------------------------
  applyDetailVisibility();
  reflectPhase();
  reflectAbort();
  composer.refreshSendButton();

  // ponytail: syntax highlighting (highlight.js, cargado por
  // CDN en standalone.templ con defer). Si ya cargó al boot
  // (cache caliente), highlightAll procesa todos los code
  // blocks del snapshot server-rendered. Si todavía no cargó,
  // sale silencioso — los SSE updates que lleguen después
  // aplican highlight por nodo en applyFragment, así que
  // igual veremos colores en los nuevos mensajes. Cuando
  // hljs termine de cargar, podemos aplicar al snapshot
  // inicial con un event listener. Por simplicidad y porque
  // los snapshots iniciales son raros (usualmente sólo el
  // prompt del user), dejamos el caso "highlight tardío del
  // snapshot" sin cubrir — la primera respuesta del agent
  // ya lo dispara.
  if (state.sessionId) {
    // highlightAll retorna false si hljs no está listo todavía.
    // Si no lo está, los applyFragment subsiguientes lo
    // pintarán.
    feed.highlightAll();
  }

  // ponytail: scroll-to-bottom inicial. Cuando abrís una sesión
  // existente, el server rinde server-side el feed completo en
  // #v2-feed (state.ConversationItems via RenderItem). Sin un
  // scroll explícito al fondo, el viewport queda arriba y el
  // user ve el principio de la conversación — tiene que
  // scrollear manualmente. El sticky scroll de maybeScrollToBottom
  // sólo se dispara cuando llega un NUEVO fragment vía SSE;
  // para historial puro (sesión cerrada, sin streaming activo)
  // jamás se llamaba.
  //
  // Hacemos DOS scrolls: uno inmediato (el contenido SSR ya
  // está en el DOM) y uno en el siguiente requestAnimationFrame
  // para cubrir layout shifts asíncronos (hljs tarda en pintar,
  // images late-load, fonts que cambian métricas).
  //
  // CORRECCIÓN: scrolleamos el WRAP, no el inner. El inner
  // (#v2-feed) tiene display:flex y crece con su contenido;
  // el wrap (#v2-feed-wrap) tiene overflow-y: auto y es el que
  // realmente scrollea cuando el contenido excede el alto
  // disponible. forceScrollToBottom contra el inner era no-op.
  if (state.sessionId && feedWrapEl && feedEl && feedEl.querySelector(".v2-item")) {
    forceScrollToBottom(feedWrapEl);
    requestAnimationFrame(() => {
      forceScrollToBottom(feedWrapEl);
    });
  }

  if (state.sessionId) {
    keepAlive(state, handlers).catch((err) => {
      console.warn("[v2] keepAlive terminated", err);
    });
  } else {
    // ponytail: sin sesión activa, el primario muestra
    // 'sin sesión' como texto informativo. Pasamos por
    // la lógica unificada (lastPrimaryLabel) para que la
    // primera pintada no genere re-paint posterior.
    if (phaseBadge) {
      lastPrimaryLabel = "sin sesión";
      lastPrimaryState = "idle";
      if (phaseBadgeText) phaseBadgeText.textContent = "sin sesión";
      phaseBadge.dataset.state = "idle";
    }
  }
}

// --- POST prompt -------------------------------------------------------
// ponytail: lanza un Error si el POST falla (status >= 400 o
// network). El caller (onSubmit del composer) usa try/catch
// para hacer rollback del setPhase optimista. Antes sólo
// console.warn y seguía como si nada, lo que dejaba el badge
// en "Trabajando" cuando el server en realidad rechazó el
// prompt.
async function postPrompt(text) {
  const root = document.getElementById("v2-app");
  const sessionId = root?.dataset.activeSessionId || "";
  if (!sessionId) throw new Error("sin sesión activa");
  const response = await fetch(`/agent/sessions/${encodeURIComponent(sessionId)}/prompt`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ message: text }),
  });
  if (!response.ok) {
    const detail = await safeErrorText(response);
    throw new Error(`prompt failed: ${detail}`);
  }
}

async function safeErrorText(response) {
  try {
    const contentType = response.headers.get("content-type") || "";
    if (contentType.includes("application/json")) {
      const body = await response.json();
      return body?.detail || body?.message || `${response.status}`;
    }
    const text = await response.text();
    return text || `${response.status}`;
  } catch {
    return `${response.status}`;
  }
}
