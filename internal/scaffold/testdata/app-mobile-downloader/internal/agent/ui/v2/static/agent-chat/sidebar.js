// sidebar.js — lista de sesiones, redirect, delete, create.
//
// La sidebar es server-rendered en el HTML inicial: el cliente
// sólo agrega interactividad (delete, crear nueva sesión, click en
// una sesión para cambiar). El listado inicial viene del backend
// vía PageState.Sessions — no lo re-fetchamos en M-A.

export function wireSidebar(rootEl, { onDelete, onCreate }) {
  if (!rootEl) return;
  rootEl.addEventListener("click", (e) => {
    const target = e.target;
    if (!(target instanceof Element)) return;
    const deleteBtn = target.closest("button[data-session-delete]");
    if (deleteBtn instanceof HTMLButtonElement) {
      e.preventDefault();
      e.stopPropagation();
      const sessionID = deleteBtn.dataset.sessionDelete || "";
      if (!sessionID) return;
      onDelete?.(sessionID);
      return;
    }
  });

  // ponytail: el form de "nueva sesión" vive en la sidebar.
  // El HTML ya tiene los inputs title/cwd/model como hidden, pero
  // el JS nunca los lee ni hace el POST. Sin este handler, el
  // botón "+" del sidebar no hace nada — un bug claro de M-A.
  // Capturamos el submit, armamos el body JSON, y mandamos al
  // endpoint V2 (que reenvía al V1 con registro de renderer).
  const newSessionForm = document.getElementById("v2-new-session");
  if (newSessionForm instanceof HTMLFormElement) {
    newSessionForm.addEventListener("submit", async (e) => {
      e.preventDefault();
      const button = newSessionForm.querySelector("button[type=submit]");
      if (button instanceof HTMLButtonElement) button.disabled = true;
      try {
        const ok = await onCreate?.(newSessionForm);
        if (!ok && button instanceof HTMLButtonElement) {
          button.disabled = false;
        }
      } catch (_) {
        if (button instanceof HTMLButtonElement) button.disabled = false;
      }
    });
  }
}

// deleteSession pide DELETE al backend. Si la respuesta es OK,
// redirige al dashboard (/agent) para que el server reevalúe
// el redirect heuristic con la nueva lista de sesiones.
export async function deleteSession(sessionID) {
  if (!sessionID) return false;
  if (!window.confirm("¿Eliminar esta sesión y su workspace?")) return false;
  try {
    const response = await fetch(`/agent/sessions/${encodeURIComponent(sessionID)}`, {
      method: "DELETE",
      headers: { Accept: "application/json" },
    });
    if (!response.ok) {
      const text = await response.text();
      throw new Error(`delete ${response.status}: ${text}`);
    }
    // Si borramos la sesión activa, vamos al dashboard. Si era
    // otra, refrescamos in-place para que la lista se actualice.
    const activeID = document.getElementById("v2-app")?.dataset.activeSessionId;
    if (activeID === sessionID) {
      window.location.href = "/agent";
    } else {
      window.location.reload();
    }
    return true;
  } catch (err) {
    console.warn("[v2] delete failed", err);
    return false;
  }
}

// ponytail: refresca el preview del último mensaje del assistant
// en el item del sidebar que corresponde a la sesión activa. Lo
// llamamos desde main.js cuando llega un turn_end. El server
// guarda el preview en Session.LastPreview cuando el assistant
// emite un message / tool / error. Hacer un GET in-place evita
// recargar la sidebar entera (que es disruptivo si el usuario
// está scrolleando por la lista).
//
// Si la sesión activa no es la que estamos viendo, no hacemos
// nada (el preview se actualizará la próxima vez que el usuario
// navegue a ella). Si la request falla, salimos silencioso —
// el preview puede quedar stale hasta el próximo refresh
// manual.
export async function refreshSidebarPreview(sessionID) {
  if (!sessionID) return;
  const item = document.querySelector(
    `#v2-sessions-list a[href*="session=${encodeURIComponent(sessionID)}"]`,
  );
  if (!item) return;
  try {
    const response = await fetch(
      `/agent/sessions/${encodeURIComponent(sessionID)}`,
      { headers: { Accept: "application/json" } },
    );
    if (!response.ok) return;
    const payload = await response.json();
    const preview = payload?.session?.lastPreview || "";
    const previewText = preview.trim().split(/\s+/).slice(0, 12).join(" ");
    const truncated =
      previewText.length > 80 ? previewText.slice(0, 79) + "…" : previewText;
    let previewEl = item.querySelector(".v2-session-preview");
    if (!truncated) {
      if (previewEl) previewEl.remove();
      return;
    }
    if (!previewEl) {
      previewEl = document.createElement("div");
      previewEl.className = "v2-session-preview";
      // Insertar después de v2-session-meta.
      const meta = item.querySelector(".v2-session-meta");
      if (meta && meta.nextSibling) {
        item.insertBefore(previewEl, meta.nextSibling);
      } else if (meta) {
        meta.after(previewEl);
      } else {
        item.appendChild(previewEl);
      }
    }
    previewEl.textContent = truncated;
  } catch (err) {
    console.warn("[v2] refreshSidebarPreview failed", err);
  }
}

// createSession lee los hidden inputs del form, hace POST al
// backend y redirige al dashboard con la nueva sesión como
// activa. El backend devuelve {session: {id, ...}} y nosotros
// armamos la URL final.
//
// Devuelve true si el POST salió bien (incluso si la redirección
// falla —我们已经得到了 el ID). false en caso de error de red o
// respuesta no-OK.
export async function createSession(formEl) {
  if (!formEl) return false;
  const body = {};
  for (const [k, v] of new FormData(formEl).entries()) {
    body[k] = typeof v === "string" ? v.trim() : v;
  }
  try {
    const response = await fetch("/agent/sessions", {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify(body),
    });
    if (!response.ok) {
      const text = await response.text();
      throw new Error(`create ${response.status}: ${text}`);
    }
    const payload = await response.json();
    const sessionID = payload?.session?.id;
    if (!sessionID) {
      throw new Error("create: response sin session.id");
    }
    // Cerrar el sidebar en mobile antes de redirigir.
    const toggle = document.getElementById("agent-sidebar");
    if (toggle && !toggle.checked) {
      // En mobile, sidebar abierta → cerrar antes de navegar.
      const isDesktop = window.matchMedia("(min-width: 1280px)").matches;
      if (!isDesktop) {
        toggle.checked = false;
      }
    }
    window.location.href = `/agent?session=${encodeURIComponent(sessionID)}`;
    return true;
  } catch (err) {
    console.warn("[v2] create failed", err);
    return false;
  }
}
