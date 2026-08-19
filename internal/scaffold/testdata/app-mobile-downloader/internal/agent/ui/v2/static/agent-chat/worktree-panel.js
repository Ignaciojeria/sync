// worktree-panel.js — gestiona el Worktree Inspector Panel.
//
// Responsabilidades:
//   - Toggle del panel (boton en el header).
//   - Fetch del snapshot al abrir (y refresco periodico).
//   - Render del branch info + files + commits.
//   - Click en file -> fetch del diff + display.
//   - Click en commit -> copy hash al clipboard.
//
// NO decide nada del chat. Solo lee metadata del worktree y la
// renderiza en el panel. Estado puro: si el server cambia, el
// panel se actualiza en el siguiente refresh.

const REFRESH_INTERVAL_MS = 5000;

export function createWorktreePanel(root, sessionId, fetchImpl = fetch) {
  const panel = document.getElementById("v2-worktree-panel");
  const toggleBtn = document.getElementById("v2-worktree-toggle");
  const closeBtn = document.getElementById("v2-worktree-close");
  const branchInfoEl = document.getElementById("v2-worktree-branch-info");
  const filesCountEl = document.getElementById("v2-worktree-files-count");
  const filesListEl = document.getElementById("v2-worktree-files");
  const commitsCountEl = document.getElementById("v2-worktree-commits-count");
  const commitsListEl = document.getElementById("v2-worktree-commits");
  const diffEl = document.getElementById("v2-worktree-diff");
  const diffContentEl = document.getElementById("v2-worktree-diff-content");
  // ponytail: elementos del botón "Apply to main". El botón se
  // habilita solo cuando hay commits ahead. Al click, hace POST
  // al endpoint /merge y muestra el resultado (ok o error).
  const applyBtn = document.getElementById("v2-worktree-apply");
  const applyStatusEl = document.getElementById("v2-worktree-apply-status");

  if (!panel || !toggleBtn) {
    return { open: () => {}, close: () => {}, refresh: async () => {} };
  }

  let open = false;
  let refreshTimer = null;
  let currentSnapshot = null;

  function show() {
    panel.hidden = false;
    toggleBtn.setAttribute("aria-pressed", "true");
    open = true;
    refresh();
    if (!refreshTimer) {
      refreshTimer = setInterval(refresh, REFRESH_INTERVAL_MS);
    }
  }

  function hide() {
    panel.hidden = true;
    toggleBtn.setAttribute("aria-pressed", "false");
    open = false;
    if (refreshTimer) {
      clearInterval(refreshTimer);
      refreshTimer = null;
    }
  }

  function toggle() {
    if (open) hide();
    else show();
  }

  async function refresh() {
    if (!sessionId) return;
    try {
      const res = await fetchImpl(
        `/agent/sessions/${encodeURIComponent(sessionId)}/worktree`,
        { headers: { Accept: "application/json" } },
      );
      if (!res.ok) return;
      const snap = await res.json();
      currentSnapshot = snap;
      render(snap);
    } catch (err) {
      console.warn("[v2] worktree refresh failed", err);
    }
  }

  function render(snap) {
    if (!snap) return;
    // ponytail: branch info. Mostramos el branch name, base branch,
    // y los commits ahead/behind. Si no hay nada, "Sin cambios".
    const ahead = Number(snap.commits_ahead) || 0;
    const behind = Number(snap.commits_behind) || 0;
    const branch = String(snap.branch || "").trim();
    const base = String(snap.base_branch || "main").trim();
    let summary = "";
    if (branch) {
      summary = branch;
      if (ahead > 0 || behind > 0) {
        summary += ` (base: ${base})`;
        if (ahead > 0) summary += ` · +${ahead}`;
        if (behind > 0) summary += ` / -${behind}`;
      }
    } else {
      summary = "Sin worktree";
    }
    if (branchInfoEl) branchInfoEl.textContent = summary;

    // ponytail: habilita el botón "Apply to main" solo si hay
    // commits ahead. Sin cambios → no hay nada que mergear.
    if (applyBtn) {
      applyBtn.disabled = ahead <= 0;
    }

    // Files list.
    const files = Array.isArray(snap.files) ? snap.files : [];
    if (filesCountEl) {
      const added = Number(snap.lines_added) || 0;
      const removed = Number(snap.lines_removed) || 0;
      filesCountEl.textContent = files.length
        ? `(${files.length} · +${added} −${removed})`
        : "(vacío)";
    }
    if (filesListEl) {
      filesListEl.innerHTML = "";
      for (const f of files) {
        const li = document.createElement("li");
        li.className = "v2-worktree-file";
        const status = String(f.status || "?").substring(0, 1);
        li.innerHTML = `
          <span class="v2-worktree-file-status" data-status="${status}">${status}</span>
          <span class="v2-worktree-file-path" title="${escapeAttr(f.path)}">${escapeText(f.path)}</span>
          <span class="v2-worktree-file-stats">
            ${Number(f.additions) > 0 ? `<span class="additions">+${f.additions}</span> ` : ""}
            ${Number(f.deletions) > 0 ? `<span class="deletions">−${f.deletions}</span>` : ""}
          </span>
        `;
        li.addEventListener("click", () => loadDiff(f.path));
        filesListEl.appendChild(li);
      }
    }

    // Commits list.
    const commits = Array.isArray(snap.commits) ? snap.commits : [];
    if (commitsCountEl) {
      commitsCountEl.textContent = commits.length ? `(${commits.length})` : "";
    }
    if (commitsListEl) {
      commitsListEl.innerHTML = "";
      for (const c of commits) {
        const li = document.createElement("li");
        li.className = "v2-worktree-commit";
        li.innerHTML = `
          <span class="v2-worktree-commit-hash">${escapeText(c.short_hash || "")}</span>
          <span class="v2-worktree-commit-message" title="${escapeAttr(c.message || "")}">${escapeText(c.message || "")}</span>
        `;
        li.addEventListener("click", () => copyCommitHash(c.hash));
        commitsListEl.appendChild(li);
      }
    }
  }

  async function loadDiff(filePath) {
    if (!sessionId || !filePath) return;
    if (!diffEl || !diffContentEl) return;
    diffEl.hidden = false;
    diffContentEl.textContent = "Cargando diff…";
    try {
      const res = await fetchImpl(
        `/agent/sessions/${encodeURIComponent(sessionId)}/worktree/diff?file=${encodeURIComponent(filePath)}`,
        { headers: { Accept: "application/json" } },
      );
      if (!res.ok) {
        diffContentEl.textContent = `Error ${res.status}`;
        return;
      }
      const data = await res.json();
      diffContentEl.textContent = data.content || "(diff vacío)";
    } catch (err) {
      diffContentEl.textContent = `Error: ${err && err.message ? err.message : err}`;
    }
  }

  async function copyCommitHash(hash) {
    if (!hash) return;
    try {
      await navigator.clipboard.writeText(hash);
      console.log("[v2] commit hash copied:", hash);
    } catch (err) {
      console.warn("[v2] copy commit hash failed", err);
    }
  }

  // ponytail: applyMerge hace POST al endpoint /merge y muestra
  // el resultado. Si el merge es OK, refresca el panel (los
  // commits_ahead van a 0 y el botón se deshabilita). Si hay
  // error (ej. merge conflicts), muestra el mensaje con detalle
  // para que el user sepa qué hacer (resolver conflictos, etc).
  async function applyMerge() {
    if (!sessionId || !applyBtn) return;
    if (!window.confirm("¿Aplicar el worktree a la rama principal? Esta acción no se puede deshacer.")) {
      return;
    }
    applyBtn.disabled = true;
    const originalText = applyBtn.textContent;
    applyBtn.textContent = "Aplicando…";
    hideApplyStatus();
    try {
      const res = await fetchImpl(
        `/agent/sessions/${encodeURIComponent(sessionId)}/merge`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json", Accept: "application/json" },
          body: "{}",
        },
      );
      if (!res.ok) {
        const text = await res.text();
        showApplyStatus("error", `Merge falló (${res.status}): ${text}`);
        return;
      }
      const data = await res.json();
      const commit = String(data.commit || "").substring(0, 7);
      showApplyStatus(
        "ok",
        `Merge OK en ${commit}. El branch ya esta en ${data.baseBranch || "main"}.`,
      );
      await refresh();
    } catch (err) {
      showApplyStatus("error", `Error de red: ${err && err.message ? err.message : err}`);
    } finally {
      applyBtn.textContent = originalText;
      if (applyBtn && currentSnapshot && (currentSnapshot.commits_ahead || 0) > 0) {
        applyBtn.disabled = false;
      }
    }
  }

  function showApplyStatus(state, message) {
    if (!applyStatusEl) return;
    applyStatusEl.hidden = false;
    applyStatusEl.dataset.state = state;
    applyStatusEl.textContent = message;
  }

  function hideApplyStatus() {
    if (!applyStatusEl) return;
    applyStatusEl.hidden = true;
    applyStatusEl.textContent = "";
    delete applyStatusEl.dataset.state;
  }

  if (applyBtn) {
    applyBtn.addEventListener("click", applyMerge);
  }

  toggleBtn.addEventListener("click", toggle);
  if (closeBtn) closeBtn.addEventListener("click", hide);

  return {
    open: show,
    close: hide,
    refresh,
  };
}

function escapeText(s) {
  return String(s ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
}

function escapeAttr(s) {
  return escapeText(s).replaceAll('"', "&quot;");
}
