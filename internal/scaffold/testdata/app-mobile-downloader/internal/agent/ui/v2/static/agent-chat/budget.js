// budget.js — gestiona la barra de presupuesto del chat V2.
//
// Responsabilidades:
//   - Lee el snapshot inicial del DOM (server-rendered) para evitar
//     parpadeo en el primer paint.
//   - Polls cada 60s el balance global desde /gateway/balance.
//   - Polls cada 15s el costo + request count desde
//     /gateway/session-cost?session_id=X.
//   - Aplica los umbrales de color (70 % → warning, 100 % → error)
//     vía data-state en el contenedor.
//
// El estado "empty" (sin datos) lo respeta el server-rendered; el
// cliente sólo cambia a "ok" / "warning" / "error" cuando tiene
// datos válidos. Si el gateway devuelve error, conservamos el
// último valor conocido y no parpadeamos a "—".

import { formatUSD } from "./dom.js";

const BALANCE_INTERVAL_MS = 60_000;
const SESSION_COST_INTERVAL_MS = 15_000;

export function createBudget({ root, sessionId, fetchImpl = fetch }) {
  const bar = root.querySelector("#v2-budget-bar");
  if (!bar) {
    return { refresh: async () => {} };
  }
  const balanceEl = bar.querySelector("#v2-balance");
  const sessionSpentEl = bar.querySelector("#v2-session-spent");
  const sessionRequestsEl = bar.querySelector("#v2-session-requests");
  // ponytail: badge del modelo + display del % de contexto.
  // Se actualizan junto con el session-cost cada 15s. El % se
  // formatea compacto (estilo pi CLI: 19k/1M (2%)) y el color
  // cambia según umbrales via data-state (ok/warning/error).
  const sessionModelEl = bar.querySelector("#v2-session-model");
  const contextPctEl = bar.querySelector("#v2-context-pct");

  // Estado interno del último valor conocido. Si el gateway falla,
  // conservamos esto y no rompemos la barra.
  let lastBalanceUSD = readSnapshotBalance(balanceEl);
  let lastSessionSpent = readSnapshotSessionSpent(sessionSpentEl);
  let lastSessionReqs = readSnapshotSessionReqs(sessionRequestsEl);
  // ponytail: también cacheamos el último modelo + context window
  // + tokens para evitar flashes cuando el gateway falla
  // momentáneamente.
  let lastModelAlias = sessionModelEl ? sessionModelEl.textContent.trim() : "";
  let lastContextWindow = 0;
  let lastPromptTokens = 0;
  let lastCompletionTokens = 0;
  let lastTotalTokens = 0;
  let balanceTimer = null;
  let sessionCostTimer = null;

  function paint() {
    if (balanceEl) {
      balanceEl.textContent =
        lastBalanceUSD == null ? "—" : formatUSD(lastBalanceUSD);
    }
    if (sessionSpentEl) {
      sessionSpentEl.textContent =
        lastSessionSpent == null ? "—" : formatUSD(lastSessionSpent);
    }
    if (sessionRequestsEl) {
      sessionRequestsEl.textContent =
        lastSessionReqs == null ? "0 req" : `${formatInt(lastSessionReqs)} req`;
    }
    // ponytail: badge del modelo + % de contexto. Como ahora son
    // spans inline al lado del Working badge (no un cell
    // separado), no necesitamos ocultar un cell — simplemente
    // actualizamos los textos cuando hay datos. El render
    // server-side ya decide si mostrar el bloque o no (vía el if
    // en page.templ con SessionModelAlias/SessionContextWindow).
    if (sessionModelEl && lastModelAlias) {
      sessionModelEl.textContent = lastModelAlias;
    }
    if (contextPctEl && lastContextWindow > 0) {
      contextPctEl.textContent = formatContextText(
        lastTotalTokens,
        lastContextWindow,
      );
      contextPctEl.dataset.state = contextState(
        lastTotalTokens,
        lastContextWindow,
      );
      contextPctEl.title = `prompt: ${formatInt(lastPromptTokens)} · completion: ${formatInt(lastCompletionTokens)}`;
    }
    paintThreshold();
  }

  // formatContextText formatea el % de contexto compacto estilo
  // pi CLI: "19k/1M (2%)". Si los tokens son < 1000 muestra
  // números enteros sin sufijo.
  function formatContextText(tokens, window) {
    if (!window || window <= 0) return "—";
    const pct = Math.min(100, Math.max(0, Math.round((tokens / window) * 100)));
    return `${formatTokensCompact(tokens)}/${formatTokensCompact(window)} (${pct}%)`;
  }

  // contextState devuelve "ok" (< 50%), "warning" (50-80%) o
  // "error" (> 80%) según el % del context window consumido.
  function contextState(tokens, window) {
    if (!window || window <= 0) return "ok";
    const pct = (tokens / window) * 100;
    if (pct >= 80) return "error";
    if (pct >= 50) return "warning";
    return "ok";
  }

  // formatTokensCompact: 847 → "847", 15420 → "15k", 1000000 → "1M".
  function formatTokensCompact(n) {
    n = Math.max(0, Math.floor(n || 0));
    if (n < 1000) return formatInt(n);
    if (n < 1000000) {
      if (n % 1000 === 0) return `${n / 1000}k`;
      return `${(n / 1000).toFixed(1).replace(/\.0$/, "")}k`;
    }
    if (n % 1000000 === 0) return `${n / 1000000}M`;
    return `${(n / 1000000).toFixed(1).replace(/\.0$/, "")}M`;
  }

  // Umbrales: 70 % del balance consumido → warning. 100 % → error.
  // Si el balance es null (gateway caído) o el session_spent es null,
  // mantenemos el estado actual.
  function paintThreshold() {
    if (lastBalanceUSD == null || lastBalanceUSD <= 0) return;
    const spent = lastSessionSpent ?? 0;
    if (spent >= lastBalanceUSD) {
      bar.dataset.state = "error";
    } else if (spent >= lastBalanceUSD * 0.7) {
      bar.dataset.state = "warning";
    } else {
      bar.dataset.state = "ok";
    }
  }

  async function refreshBalance() {
    try {
      const res = await fetchImpl("/gateway/balance?format=json", {
        headers: { Accept: "application/json" },
      });
      if (!res.ok) return;
      const body = await res.json();
      const v = Number(body?.balance_usd);
      if (Number.isFinite(v)) {
        lastBalanceUSD = v;
        paint();
      }
    } catch (err) {
      console.warn("[v2] balance poll failed", err);
    }
  }

  async function refreshSessionCost() {
    if (!sessionId) return;
    try {
      const url = `/gateway/session-cost?session_id=${encodeURIComponent(sessionId)}`;
      const res = await fetchImpl(url, { headers: { Accept: "application/json" } });
      if (!res.ok) return;
      const body = await res.json();
      const cost = Number(body?.estimated_cost_usd);
      const reqs = Number(body?.request_count);
      if (Number.isFinite(cost)) lastSessionSpent = cost;
      if (Number.isFinite(reqs)) lastSessionReqs = reqs;
      // ponytail: también leemos model_alias + context_window +
      // tokens del response del gateway. Si vienen vacíos,
      // mantenemos el último valor conocido para evitar flash.
      const modelAlias = String(body?.model_alias || "").trim();
      const contextWindow = Number(body?.context_window) || 0;
      const promptTokens = Number(body?.prompt_tokens) || 0;
      const completionTokens = Number(body?.completion_tokens) || 0;
      const totalTokens = Number(body?.total_tokens) || 0;
      if (modelAlias) lastModelAlias = modelAlias;
      if (contextWindow > 0) lastContextWindow = contextWindow;
      lastPromptTokens = promptTokens;
      lastCompletionTokens = completionTokens;
      lastTotalTokens = totalTokens;
      paint();
    } catch (err) {
      console.warn("[v2] session-cost poll failed", err);
    }
  }

  function start() {
    stop();
    // ponytail: la barra muestra el snapshot server-rendered
    // inmediatamente, pero hacemos el primer poll apenas arranca
    // el módulo para tener datos frescos. Sin esto, los valores
    // quedan viejos hasta el primer intervalo.
    if (sessionId) {
      refreshSessionCost();
      sessionCostTimer = setInterval(refreshSessionCost, SESSION_COST_INTERVAL_MS);
    }
    refreshBalance();
    balanceTimer = setInterval(refreshBalance, BALANCE_INTERVAL_MS);
  }

  function stop() {
    if (balanceTimer) clearInterval(balanceTimer);
    if (sessionCostTimer) clearInterval(sessionCostTimer);
    balanceTimer = null;
    sessionCostTimer = null;
  }

  paint();
  return { start, stop, refresh: refreshSessionCost };
}

// --- snapshot helpers ---------------------------------------------------

// readSnapshotBalance lee el balance inicial desde el texto del DOM.
// Devuelve null si el server-rendered dice "—" o si el formato no
// matchea un número USD.
function readSnapshotBalance(el) {
  if (!el) return null;
  const text = el.textContent || "";
  if (text.trim() === "—") return null;
  return parseUSD(text);
}

// readSnapshotSessionSpent y readSnapshotSessionReqs son iguales en
// forma, separados para claridad. El session_spent puede ser null
// si la sesión es nueva y el gateway aún no sabe de ella.
function readSnapshotSessionSpent(el) {
  if (!el) return null;
  const text = el.textContent || "";
  if (text.trim() === "—") return null;
  return parseUSD(text);
}

function readSnapshotSessionReqs(el) {
  if (!el) return null;
  const text = el.textContent || "";
  const m = text.match(/(\d+)/);
  return m ? Number(m[1]) : null;
}

function parseUSD(text) {
  const cleaned = String(text).replace(/[^0-9.\-]/g, "");
  const n = Number(cleaned);
  return Number.isFinite(n) ? n : null;
}

function formatInt(n) {
  return new Intl.NumberFormat("en-US").format(Math.max(0, Math.floor(n)));
}
