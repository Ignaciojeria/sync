// composer.js — gestiona el input y el envío de prompts.
//
// Reglas:
//   - Enter envía, Shift+Enter inserta newline
//   - IME guard: Enter durante composición no envía
//   - Botón Enviar se habilita sólo si hay texto y no estamos
//     en medio de un turno
//   - Submit hace UI optimista (appendUser) + POST. Si el POST
//     falla, muestra error y revierte el mensaje.
//   - Autosize: el textarea crece con el contenido desde
//     MIN_LINES hasta MAX_LINES (después hace scroll interno).
//
// El estado "phase running" lo refleja el main.js (deshabilita el
// input). composer.js sólo lee `canSend` para no duplicar lógica.

import { escapeHTML } from "./dom.js";

// ponytail: límites del autosize. MIN_LINES es la altura
// inicial (1 fila visible sin padding). MAX_LINES es el tope
// antes de activar scroll interno — más allá el textarea
// deja de crecer y el usuario hace scroll. Estos límites
// están en el JS (no en CSS) porque necesitamos medir el
// line-height del font actual para calcular píxeles.
const MIN_LINES = 1;
const MAX_LINES = 8;
const LINE_HEIGHT_PX = 21; // 0.92rem * 1.5 = 1.38rem ≈ 22px en la mayoría de browsers
const VERTICAL_PADDING_PX = 22; // padding-y (0.65rem) + border (1px) * 2

export function createComposer({ formEl, inputEl, sendButton, state, callbacks }) {
  // ponytail: autosize reactivo al contenido. Cada vez que
  // cambia el valor del textarea, recalculamos el height en
  // función del contenido (scrollHeight) clampeado entre
  // min y max. El truco: primero reseteamos height a 'auto'
  // para que scrollHeight refleje el contenido natural, y
  // después ponemos el height final.
  function autosize() {
    if (!inputEl) return;
    inputEl.style.height = "auto";
    const min = MIN_LINES * LINE_HEIGHT_PX + VERTICAL_PADDING_PX;
    const max = MAX_LINES * LINE_HEIGHT_PX + VERTICAL_PADDING_PX;
    const next = Math.min(max, Math.max(min, inputEl.scrollHeight));
    inputEl.style.height = `${next}px`;
    // ponytail: si llegamos al máximo, activamos scroll
    // interno (overflow-y:auto). Si no, lo dejamos en hidden
    // para que no aparezca scrollbar entre crecimientos.
    inputEl.style.overflowY = next >= max ? "auto" : "hidden";
  }

  function refreshSendButton() {
    if (!sendButton) return;
    const hasText = (inputEl?.value || "").trim().length > 0;
    sendButton.disabled = !hasText || state.phase === "running" || !state.sessionId;
  }

  // Guardamos un listener para Enter-to-send que respeta IME.
  if (inputEl) {
    inputEl.addEventListener("input", () => {
      refreshSendButton();
      autosize();
    });
    inputEl.addEventListener("keydown", (e) => {
      if (e.key !== "Enter") return;
      if (e.shiftKey) return;
      if (e.isComposing || e.keyCode === 229) return;
      if (inputEl.disabled || inputEl.readOnly) return;
      e.preventDefault();
      formEl?.requestSubmit();
    });
    // ponytail: ajuste inicial después de que el browser
    // hidrate el textarea (en caso de que el server lo
    // prellene, por ejemplo al restaurar un draft). El
    // requestAnimationFrame espera al próximo frame para
    // que scrollHeight sea preciso.
    requestAnimationFrame(autosize);
  }

  if (formEl) {
    formEl.addEventListener("submit", async (e) => {
      e.preventDefault();
      const text = (inputEl?.value || "").trim();
      if (!text) return;
      if (!state.sessionId) return;

      inputEl.value = "";
      refreshSendButton();
      autosize();
      await callbacks.onSubmit(text);
      inputEl?.focus();
    });
  }

  // Lock / unlock del composer mientras hay un turno corriendo.
  function setLocked(locked) {
    if (inputEl) inputEl.disabled = !!locked;
    refreshSendButton();
  }

  return { refreshSendButton, setLocked, autosize };
}

// appendUser es la versión "pura" que usa composer cuando hace
// UI optimista. La exportamos porque main.js también la invoca en
// tests. Mismo HTML que feed.js:appendUser — si cambia el wrapper
// hay que actualizar los dos lugares.
export function buildUserItemHTML(text) {
  return `<div class="v2-item v2-item-user" data-kind="user">${escapeHTML(text)}</div>`;
}
