import type { ExtensionAPI } from "@mariozechner/pi-coding-agent";
import { basename } from "node:path";

function currentSessionID(ctx: { sessionManager?: { getSessionFile?: () => string | undefined } }) {
  const file = ctx.sessionManager?.getSessionFile?.();
  if (!file) return "";
  return basename(file).replace(/\.(jsonl|json)$/i, "");
}

export default function (pi: ExtensionAPI) {
  const baseUrl = process.env.SYNC_AI_GATEWAY_BASE_URL || "https://sync-ai-gateway.exe.xyz/api/gateway/v1";
  const apiKey = process.env.SYNC_AI_GATEWAY_API_KEY || "sag_d71a360ef0491d3a71a531a65ae229693b36d5f9f69aa5a4";

  pi.registerProvider("sync-ai-gateway", {
    baseUrl,
    apiKey,
    authHeader: true,
    api: "openai-completions",
    models: [
      {
        id: "minimax/m3",
        name: "MiniMax M3",
        reasoning: false,
        input: ["text", "image"],
        cost: { input: 0.30, output: 1.20, cacheRead: 0.06, cacheWrite: 0 },
        contextWindow: 128000,
        maxTokens: 4096,
        compat: {
          supportsDeveloperRole: false,
          supportsUsageInStreaming: true,
          maxTokensField: "max_tokens",
        },
      },
    ],
  });

  pi.on("session_start", async (_event, ctx) => {
    const sessionID = currentSessionID(ctx);
    if (sessionID) {
      pi.registerProvider("sync-ai-gateway", {
        baseUrl,
        apiKey,
        authHeader: true,
        headers: {
          "X-SyncAI-Session-ID": sessionID,
        },
      });
    }

    const model = ctx.modelRegistry.find("sync-ai-gateway", "minimax/m3");
    if (model) await pi.setModel(model);
  });
}