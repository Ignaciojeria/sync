// feed.js — gestiona el contenedor de mensajes.
//
// Responsabilidades:
//   - upsert por data-msg (cuando llega un fragment streaming, reemplaza
//     el bloque anterior para que la burbuja "crezca")
//   - auto-scroll al fondo en cada update
//   - detectar el "empty state" para mostrar el hero cuando no hay
//     items
//   - aplicar syntax highlighting (highlight.js) a los code blocks
//     que llegan en cada fragment
//
// NO decide nada sobre el phase. Sólo aplica lo que recibe de sse.js.

import {
  applyServerHtml,
  forceScrollToBottom,
  isNearBottom,
  maybeScrollToBottom,
} from "./dom.js";

// ponytail: highlight.js vive en una variable global (window.hljs)
// porque lo cargamos desde CDN (no queremos importar el módulo
// — suma 30KB al bundle del chat y el CDN tiene cache de browser).
// La función es defensiva: si hljs todavía no cargó (cold cache
// o red lenta), el highlight simplemente no se aplica — los code
// blocks quedan en texto plano monoespaciado, que sigue siendo
// legible y no rompe el layout.
//
// applyHighlight procesa UN container (no todo el feed) para
// no re-procesar todos los code blocks en cada SSE update.
// highlightElement es la API de v11 que detecta el lenguaje por
// la clase language-X (que goldmark ya agrega en el <code>).
function applyHighlight(container) {
  if (!container || !window.hljs) return;
  const blocks = container.querySelectorAll("pre code:not(.hljs)");
  if (!blocks.length) return;
  for (const block of blocks) {
    try {
      window.hljs.highlightElement(block);
    } catch (err) {
      console.warn("[v2] highlight failed", err);
    }
  }
}

// highlightAll procesa todo el feed. Útil al boot inicial cuando
// la página se carga con el snapshot server-rendered (los
// primeros mensajes ya vienen con el HTML listo y necesitan
// highlight).
function highlightAll() {
  if (!window.hljs) return false;
  // ponytail: highlightAll de highlight.js escanea todo el
  // documento. Lo usamos solo si no hay mensajes dinámicos
  // aún (boot). Para los SSE updates usamos applyHighlight
  // por container.
  if (typeof window.hljs.highlightAll === "function") {
    try {
      window.hljs.highlightAll();
      return true;
    } catch (err) {
      console.warn("[v2] highlightAll failed", err);
      return false;
    }
  }
  return false;
}

// ponytail: M-C.1 = botón Copy en cada item. wireCopyButtons
// se llama desde main.js al boot. Handler delegado en el feed
// (un solo listener para todos los items, como el resto del
// módulo). Lee el textContent del item (sin HTML/markdown), lo
// copia al clipboard via navigator.clipboard, y da feedback
// visual por 1.5s. Si la API moderna falla, fallback a
// document.execCommand("copy") con textarea temporal (browsers
// viejos, file://, http sin TLS).
export function wireCopyButtons(feedEl) {
  if (!feedEl) return;
  feedEl.addEventListener("click", async (e) => {
    const btn = e.target instanceof Element ? e.target.closest("[data-copy-button]") : null;
    if (!btn) return;
    e.preventDefault();
    const item = btn.closest("[data-msg]");
    if (!item) return;
    // ponytail: textContent extrae el texto visible del item,
    // ignorando las tags HTML (incluyendo el SVG del propio
    // botón Copy). Para assistant con markdown, esto da el
    // texto plano — suficiente para copy/paste. Si el usuario
    // quiere el markdown original, lo agregamos en M-C.2 con
    // un endpoint GET que devuelve el raw.
    const text = (item.textContent || "").trim();
    if (!text) return;
    const ok = await copyText(text);
    if (!ok) {
      console.warn("[v2] copy failed");
      return;
    }
    btn.dataset.state = "copied";
    btn.setAttribute("aria-label", "Copiado");
    const originalTitle = btn.getAttribute("title") || "Copiar";
    btn.setAttribute("title", "Copiado");
    setTimeout(() => {
      delete btn.dataset.state;
      btn.setAttribute("aria-label", "Copiar mensaje");
      btn.setAttribute("title", originalTitle);
    }, 1500);
  });
}

// ponytail: wireRegenerateButtons wirea los botones de Regenerate
// en items kind=assistant. Al click, llama al endpoint
// POST /agent/sessions/{id}/regenerate que:
//   1. Borra del transcript los items posteriores al último
//      user prompt (via truncateTranscriptAfter).
//   2. Re-envía el prompt al runtime.
//   3. Responde 200 OK con {clearAfterSeq: N}.
//
// El handler:
//   1. Marca el botón como "regenerating" (spinner, no
//      clickable) para feedback visual.
//   2. Hace POST al endpoint.
//   3. Cuando recibe el clearAfterSeq, borra del feed los
//      items con data-msg > clearAfterSeq (via callback onClear).
//   4. Los nuevos fragments del assistant llegan por SSE y se
//      renderizan normalmente (con seqs nuevos).
//   5. Si el POST falla, restaura el botón al estado normal.
export function wireRegenerateButtons(feedEl, sessionId, onClear) {
  if (!feedEl) return;
  feedEl.addEventListener("click", async (e) => {
    const btn = e.target instanceof Element
      ? e.target.closest("[data-regenerate-button]")
      : null;
    if (!btn) return;
    e.preventDefault();
    if (btn.dataset.state === "regenerating") return;
    btn.dataset.state = "regenerating";
    btn.setAttribute("aria-label", "Regenerando…");
    try {
      const response = await fetch(
        `/agent/sessions/${encodeURIComponent(sessionId)}/regenerate`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: "{}",
        },
      );
      if (!response.ok) {
        const text = await response.text();
        throw new Error(`regenerate ${response.status}: ${text}`);
      }
      const payload = await response.json();
      const clearAfterSeq = Number(payload?.clearAfterSeq) || 0;
      // ponytail: limpiamos el feed antes de empezar a recibir
      // los nuevos fragments. Sin esto, los items viejos del
      // assistant quedarían visibles junto a los nuevos.
      if (onClear) onClear(clearAfterSeq);
      // El botón vuelve al estado normal cuando llegue el
      // primer fragment del nuevo assistant (via applyFragment
      // que detecta el cambio). Si por alguna razón no llega,
      // un timeout de seguridad lo restaura después de 30s.
      setTimeout(() => {
        if (btn.dataset.state === "regenerating") {
          delete btn.dataset.state;
          btn.setAttribute("aria-label", "Regenerar respuesta");
        }
      }, 30000);
    } catch (err) {
      console.warn("[v2] regenerate failed", err);
      delete btn.dataset.state;
      btn.setAttribute("aria-label", "Regenerar respuesta");
    }
  });
}

async function copyText(text) {
  if (typeof navigator !== "undefined" && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch (_) {
      // cae al fallback
    }
  }
  // Fallback: textarea temporal + execCommand("copy").
  try {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.top = "-9999px";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.focus();
    ta.select();
    const ok = document.execCommand("copy");
    document.body.removeChild(ta);
    return ok;
  } catch (err) {
    console.warn("[v2] execCommand copy failed", err);
    return false;
  }
}

// ponytail: createFeed toma DOS refs distintas porque el
// contenedor del feed tiene overflow-y: auto en el WRAP
// (#v2-feed-wrap) y los items se insertan en el INNER (#v2-feed).
// Confundirlos era el bug del scroll roto: los scrollTop/scrollHeight
// se aplicaban al inner (que no scrollea, sólo crece con contenido)
// y nunca afectaban al wrap (que es lo que el user ve moverse).
//
//   scrollEl  → #v2-feed-wrap (overflow-y: auto, escucha scroll,
//                              button jump-bottom, distanceFromBottom)
//   contentEl → #v2-feed (querySelector de [data-msg], upsert por
//                          id, applyServerHtml, clearFromSeq)
//   emptyEl   → elemento que muestra el estado vacío del feed
export function createFeed(scrollEl, contentEl, emptyEl) {
  // ponytail: botón jump-to-bottom. Aparece cuando el user sube
  // a leer mensajes anteriores y el chat sigue generando contenido
  // abajo. El botón muestra un contador de "mensajes nuevos" que
  // se incrementa con cada fragment que NO se scrolleó porque el
  // user estaba arriba. Click → scroll al fondo + oculta el botón.
  const jumpButton = document.getElementById("v2-jump-bottom");
  const jumpCountEl = document.getElementById("v2-jump-bottom-count");
  let pendingNewMessages = 0;

  function showJumpToBottom() {
    if (!jumpButton) return;
    jumpButton.hidden = false;
    if (jumpCountEl) {
      // ponytail: si hay mensajes nuevos pendientes, mostramos
      // el contador ("Mensaje nuevo" / "N mensajes nuevos"). Si
      // no, dejamos el contador vacío y solo se ve la flecha
      // — el user subió a leer y la flecha le indica que puede
      // bajar con un click.
      if (pendingNewMessages > 1) {
        jumpCountEl.textContent = `${pendingNewMessages} mensajes nuevos`;
        jumpCountEl.hidden = false;
      } else if (pendingNewMessages === 1) {
        jumpCountEl.textContent = "Mensaje nuevo";
        jumpCountEl.hidden = false;
      } else {
        jumpCountEl.textContent = "";
        jumpCountEl.hidden = true;
      }
    }
  }

  function hideJumpToBottom() {
    if (!jumpButton) return;
    jumpButton.hidden = true;
    pendingNewMessages = 0;
    if (jumpCountEl) jumpCountEl.textContent = "";
  }

  function notifyNewMessage() {
    pendingNewMessages += 1;
    showJumpToBottom();
  }

  // syncJumpToBottom lo llama appendUser/applyFragment después
  // del scroll. Si por algún motivo el scroll sí fue al fondo
  // (user estaba cerca), oculta el botón y resetea el contador.
  // Si NO se scrolleó, mantiene el estado visible.
  function syncJumpToBottom() {
    if (isNearBottom(scrollEl)) {
      hideJumpToBottom();
    }
  }

  if (jumpButton) {
    jumpButton.addEventListener("click", () => {
      // ponytail: forceScrollToBottom ignora el sticky threshold
      // porque es una acción EXPLÍCITA del user. Si el feed
      // todavía está renderizando (último fragment en vuelo),
      // dejamos que llegue el siguiente — el observer se va
      // a encargar de mostrar/ocultar el botón según el estado
      // de scroll después del jump.
      forceScrollToBottom(scrollEl);
      hideJumpToBottom();
    });
  }

  // ponytail: si el user scrollea manualmente al fondo, ocultamos
  // el botón jump-to-bottom sin esperar al próximo fragment. Asi
  // el botón no queda visible si el user ya está viendo el final
  // del chat. También mostramos el botón cuando el user SUBE (no
  // está cerca del fondo) — así sabe que hay contenido abajo sin
  // tener que esperar al próximo fragment.
  if (scrollEl) {
    scrollEl.addEventListener(
      "scroll",
      () => {
        if (isNearBottom(scrollEl)) {
          hideJumpToBottom();
        } else {
          // ponytail: si el user subió, mostramos el botón
          // inmediatamente. Si ya había un contador de
          // "mensajes nuevos" lo conservamos; si no, lo dejamos
          // vacío (la flecha apunta hacia abajo sin texto).
          showJumpToBottom();
        }
      },
      { passive: true },
    );
  }

  // appendUser añade un bubble de usuario al feed. Lo usamos como
  // UI optimista: el mensaje aparece instantáneamente antes de que
  // el server confirme con SSE.
  function appendUser(text) {
    const safe = escapeHTMLSimple(text);
    const html = `<div class="v2-item v2-item-user" data-kind="user">${safe}</div>`;
    applyServerHtml(contentEl, html, null);
    setEmpty(false);
    // ponytail: scroll sticky. Si el user está cerca del fondo,
    // scrolleamos. Si está arriba, no forzamos — dejamos que lea
    // lo que estaba leyendo. Si no scrolleamos, incrementamos el
    // contador de "mensajes nuevos" del botón jump-to-bottom.
    //
    // CORRECCIÓN: scrollEl (no contentEl) es el que tiene
    // overflow-y: auto. Antes apuntábamos al inner (contentEl),
    // que no scrollea, así que el sticky trigger nunca movía nada
    // visible. Bug histórico, no sólo del fix #1 — applyFragment
    // ya tenía este problema desde v2.
    //
    // ORDEN: el scroll se hace DESPUÉS de cualquier highlight
    // (no hay highlight en bubbles de user, pero mantener el
    // mismo orden entre appendUser y applyFragment simplifica el
    // modelo mental: siempre DOM → highlight → scroll). Si en el
    // futuro bubbles de user pasan a tener code blocks (ej.
    // snippets con triple backtick), no hay que tocar nada.
    syncJumpToBottom();
    if (!maybeScrollToBottom(scrollEl)) {
      notifyNewMessage();
    }
  }

  // ponytail: M-C.2 = Regenerate. clearFromSeq borra del feed
  // todos los items con data-msg > afterSeq. Lo usa el handler
  // onRegenerate después del POST al server (que devuelve el
  // clearAfterSeq) y antes de empezar a recibir los nuevos
  // fragments del assistant. Sin esta limpieza, los items
  // viejos del assistant quedarían visibles junto a los nuevos
  // hasta el final del turno.
  function clearFromSeq(afterSeq) {
    if (!contentEl) return;
    const items = contentEl.querySelectorAll("[data-msg]");
    for (const item of items) {
      const seqStr = item.getAttribute("data-msg");
      // data-msg puede ser un número (Seq) o string (legacy
      // seq como texto). Parseamos ambos formatos.
      let seq = 0;
      try {
        seq = parseInt(seqStr, 10);
      } catch (_) {
        continue;
      }
      if (!Number.isFinite(seq) || seq <= 0) continue;
      if (seq > afterSeq) item.remove();
    }
  }

  // applyFragment hace upsert por id. Si ya hay un bloque con
  // data-msg=N lo reemplaza; si no, appendea. Después aplica
  // syntax highlighting a los nuevos code blocks del fragment.
  function applyFragment(envelope) {
    if (!envelope?.html) return;
    // ponytail: pasamos upsertKey además de id. El cliente
    // prefiere upsertKey (estable a través de los
    // tool_execution_update de un mismo tool call) y cae a id
    // si no está presente. Sin esto, una tool con N updates
    // crearía N cards separadas en lugar de reemplazar la
    // misma. El bug que el user reportó como "el chat pierde
    // mensajes" durante operaciones largas (npm install, cargo
    // build, etc.) era exactamente este: las partials se
    // descartaban y el output aparecía sólo al END, dando la
    // sensación de "se quedó pegado".
    const node = applyServerHtml(contentEl, envelope.html, envelope.id, envelope.upsertKey);
    setEmpty(false);
    // ponytail: ORDEN CORREGIDO — highlight va ANTES del scroll.
    // Antes el orden era scroll → highlight, pero applyHighlight
    // (hljs) puede cambiar el wrap de los code blocks (texto
    // styled wrappea distinto que raw text), creciendo el alto
    // del wrap. Si scrolleamos antes del highlight, el snap se
    // fija a un valor que queda corto apenas hljs aplica, y el
    // user ve el fondo "cortado" — lo que reportaba como
    // "tengo que bajar manualmente a medida que llega texto".
    // El fix es: pintar el HTML → highlight → scroll, así el
    // snap usa la altura final.
    applyHighlight(node || contentEl);
    syncJumpToBottom();
    if (!maybeScrollToBottom(scrollEl)) {
      notifyNewMessage();
    }
  }

  function setEmpty(isEmpty) {
    if (!emptyEl) return;
    emptyEl.hidden = !isEmpty;
  }

  function clear() {
    while (contentEl?.firstChild) {
      contentEl.removeChild(contentEl.firstChild);
    }
  }

  return { appendUser, applyFragment, setEmpty, clear, applyHighlight, highlightAll, clearFromSeq };
}

// escapeHTMLSimple escapa los 5 caracteres peligrosos. Duplicado
// de dom.js:escapeHTML porque acá lo queremos inline en el template
// string sin tener que importar nada (main.js lo pasa pero
// mantener este helper local reduce el acoplamiento).
function escapeHTMLSimple(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}
