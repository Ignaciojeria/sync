import type { ExtensionAPI } from "@mariozechner/pi-coding-agent";
import { appendFileSync, mkdirSync } from "node:fs";
import { join } from "node:path";

export default function (pi: ExtensionAPI) {
  pi.on("session_start", async (_event, ctx) => {
    try {
      const dir = join(ctx.cwd, ".pi", "tmp");
      mkdirSync(dir, { recursive: true });
      appendFileSync(join(dir, "smoke-extension.log"), `session_start cwd=${ctx.cwd}\n`);
    } catch {
      // smoke test: no-op
    }
  });

  pi.registerCommand("pingext", {
    description: "Smoke test extension command",
    handler: async (_args, ctx) => {
      ctx.ui.notify("smoke extension loaded", "info");
    },
  });
}