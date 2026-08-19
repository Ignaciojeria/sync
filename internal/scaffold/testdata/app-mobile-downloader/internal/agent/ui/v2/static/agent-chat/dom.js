// dom.js — helpers DOM puros, sin estado.
//
// Cualquier módulo que necesite formatear, escapar o crear
// elementos usa estos helpers. El resto del módulo NO toca el DOM
// directamente: pasa por acá. Eso permite testear las funciones
// sin tener que montar un jsdom.

// escapeHTML escapa los 5 caracteres que rompen HTML inline. Lo usa
// el cliente cuando quiere mostrar texto crudo del usuario sin
// pasar por el render server.
export function escapeHTML(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

// formatUSD formatea un número como USD. Maneja el caso < 0.01
// mostrando 4 decimales para que el badge no diga "$0.00" cuando
// el costo real fue $0.0003.
export function formatUSD(value) {
  const n = Number(value);
  if (!Number.isFinite(n)) return "—";
  if (n < 0.01) {
    return new Intl.NumberFormat("en-US", {
      style: "currency",
      currency: "USD",
      minimumFractionDigits: 4,
    }).format(n);
  }
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(n);
}

// applyServerHtml inserta HTML del server en un contenedor. Si
// upsertKey viene, intenta upsert por data-upsert-key
// (reemplaza el bloque existente). Si no, intenta upsert por
// data-msg (legacy). Si tampoco hay match, appendea al final.
//
// Devuelve el elemento insertado/reemplazado, o null si no se
// pudo.
export function applyServerHtml(container, html, msgId, upsertKey) {
  if (!html || !container) return null;
  // ponytail: upsertKey tiene prioridad sobre msgId porque es
  // estable a través de los tool_execution_update de un mismo
  // tool call (la seq del journal cambia por update, el
  // toolCallId no). Sin esta prioridad, una tool con N updates
  // crearía N cards separadas (una por seq) en vez de reemplazar
  // la misma. El match devuelve el elemento existente antes del
  // replace; el outerHTML lo reemplaza in-place; el query post
  // recupera el nodo nuevo (que ya tiene los nuevos data-*).
  let existing = null;
  if (upsertKey != null && upsertKey !== "") {
    const upSel = `[data-upsert-key="${cssEscape(String(upsertKey))}"]`;
    existing = container.querySelector(upSel);
  }
  if (!existing && msgId != null && msgId !== "") {
    const sel = `[data-msg="${cssEscape(String(msgId))}"]`;
    existing = container.querySelector(sel);
  }
  if (existing) {
    existing.outerHTML = html;
    // Recuperamos el nodo nuevo. Probamos ambos selectores
    // porque el HTML puede traer data-upsert-key o data-msg.
    if (upsertKey) {
      const found = container.querySelector(
        `[data-upsert-key="${cssEscape(String(upsertKey))}"]`
      );
      if (found) return found;
    }
    if (msgId != null && msgId !== "") {
      const sel = `[data-msg="${cssEscape(String(msgId))}"]`;
      const found = container.querySelector(sel);
      if (found) return found;
    }
    return null;
  }
  const tpl = document.createElement("template");
  tpl.innerHTML = String(html).trim();
  const node = tpl.content.firstElementChild;
  if (node) {
    container.appendChild(node);
    return node;
  }
  return null;
}

// ponytail: scroll sticky. Si el usuario está cerca del fondo
// del feed (umbral: 80px), hacemos scroll al fondo automáticamente
// cuando llega un nuevo fragment. Si está más arriba (leyendo
// contexto anterior), NO forzamos el scroll — el nuevo contenido
// aparece abajo pero el user mantiene su posición. Aparece un
// botón "jump to bottom" (gestionado por feed.js) que el user
// puede clickear para ir al fondo manualmente.
//
// Devuelve true si scrolleó, false si no (porque el user estaba
// arriba). feed.js usa el boolean para incrementar el contador
// de "mensajes nuevos" en el botón.
//
// Robustness: el snap inmediato con `scrollTop = scrollHeight`
// lee el layout actual, pero hay 3 fuentes de drift durante
// streaming:
//   1. hljs aplica syntax highlighting después (en feed.js
//      applyFragment llama a applyHighlight DESPUÉS del scroll).
//      Los code blocks styled wrappean distinto que los raw y
//      pueden crecer el alto del contenedor.
//   2. Chrome tiene `overflow-anchor: auto` por default en
//      contenedores de scroll — intenta mantener visible lo que
//      el user está leyendo ajustando scrollTop automáticamente.
//      En un chat eso es contraproducente: cuando llega texto
//      abajo (que es donde lo queremos), Chrome puede "anclar"
//      la posición previa y dejar el nuevo contenido fuera de
//      vista. Mitigamos con `overflow-anchor: none` en CSS, pero
//      también hacemos snap en doble rAF y un timeout para
//      cubrir paths donde el anchor aún así se active.
//   3. Múltiples fragments en succession (text_deltas llegan a
//      60+Hz). El scroll fijado puede ser pisado por el
//      siguiente scrollHeight si llega antes del paint. La doble
//      rAF coalesce — el navegador sólo pinta el último valor.
const STICKY_THRESHOLD_PX = 80;
export function maybeScrollToBottom(container) {
  if (!container) return false;
  const distanceFromBottom =
    container.scrollHeight - container.scrollTop - container.clientHeight;
  if (distanceFromBottom <= STICKY_THRESHOLD_PX) {
    snapToBottom(container);
    return true;
  }
  return false;
}

// ponytail: alias deprecated de scrollToBottom. Ahora el scroll
// es sticky (see maybeScrollToBottom). Lo dejamos por si algún
// módulo legacy todavía lo llama, pero su semántica cambió: ya
// no fuerza el scroll, sólo scrollea si el user está cerca del
// fondo. Si el módulo quiere forzar el scroll (ej. jump-to-bottom
// button click), debe usar forceScrollToBottom.
export function scrollToBottom(container) {
  return maybeScrollToBottom(container);
}

// forceScrollToBottom ignora el sticky threshold y scrollea
// siempre al fondo. Lo usa el botón "jump to bottom" cuando el
// user hace click explícito.
export function forceScrollToBottom(container) {
  if (!container) return;
  snapToBottom(container);
}

// ponytail: snap robusto al fondo del contenedor. Hace 4 intentos
// escalonados para tolerar layout shifts asíncronos:
//   1. Inmediato: cubre el content que ya está en el DOM.
//   2. requestAnimationFrame: cubre el próximo paint (lo que
//      cambió entre el set anterior y el próximo frame).
//   3. Doble requestAnimationFrame: cubre el caso donde el primer
//      rAF disparó UN paint pero el contenido todavía está
//      cambiando (hljs, fonts, imágenes).
//   4. setTimeout 100ms: cubre el caso slow path — imágenes
//      lazy-load que crecen después de pintarse, fonts custom
//      que tardan más de un frame en aplicar.
//
// Los 4 snaps son idempotentes (todos asignan la misma posición
// al fondo), así que sumar intentos no genera jitter — sólo
// robustez ante layouts asíncronos. La coalescencia natural de
// rAF y setTimeout evitan trabajo extra: si el próximo fragment
// llega antes, los snaps anteriores quedan en estado "ya viejo"
// y se reemplazan por el siguiente.
function snapToBottom(container) {
  if (!container) return;
  container.scrollTop = container.scrollHeight;
  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      container.scrollTop = container.scrollHeight;
      setTimeout(() => {
        container.scrollTop = container.scrollHeight;
      }, 100);
    });
  });
}

// isNearBottom devuelve true si el user está dentro del umbral
// de distancia del fondo. Útil para que feed.js sepa cuándo
// ocultar el botón jump-to-bottom (el user scrolleó manualmente
// al fondo).
export function isNearBottom(container) {
  if (!container) return true;
  const distanceFromBottom =
    container.scrollHeight - container.scrollTop - container.clientHeight;
  return distanceFromBottom <= STICKY_THRESHOLD_PX;
}

// cssEscape escapa un string para usarlo dentro de un selector
// CSS attribute. Es la versión mínima compatible con los browsers
// que soporta el repo (sin depender de CSS.escape).
export function cssEscape(value) {
  if (typeof CSS !== "undefined" && CSS.escape) return CSS.escape(value);
  return String(value).replace(/[^a-zA-Z0-9_-]/g, (c) => `\\${c}`);
}

// dispatchCustomEvent helper para emitir CustomEvents tipados. Lo
// usan los módulos para coordinarse sin tener referencias
// cruzadas.
export function dispatchCustomEvent(target, type, detail) {
  if (!target) return;
  target.dispatchEvent(new CustomEvent(type, { detail, bubbles: true }));
}
