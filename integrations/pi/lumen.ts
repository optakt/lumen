/**
 * Lumen Pi Extension
 *
 * Integrates the Lumen belief store with the Pi agent harness.
 * Before each model call, fetches the current epistemic state from
 * Lumen and injects it as a context message. After each assistant
 * turn, posts the response text to Lumen for claim extraction.
 *
 * Installation:
 *   cp lumen.ts ~/.pi/agent/extensions/lumen.ts
 *
 * Configuration (environment variables):
 *   LUMEN_URL              Lumen server base URL (default: http://localhost:3737)
 *   LUMEN_MAX_BELIEFS      Max beliefs to inject (default: 8)
 *   LUMEN_MIN_CONFIDENCE   Min confidence threshold 0–1 (default: 0.5)
 *   LUMEN_ENABLED          Set to "false" to disable without removing the file
 */

import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

const DEFAULT_URL = "http://localhost:3737";
const DEFAULT_MAX = 8;
const DEFAULT_MIN_CONF = 0.5;

export default function (pi: ExtensionAPI) {
  const baseURL = (process.env.LUMEN_URL ?? DEFAULT_URL).replace(/\/$/, "");
  const maxBeliefs = parseInt(process.env.LUMEN_MAX_BELIEFS ?? String(DEFAULT_MAX), 10);
  const minConf = parseFloat(process.env.LUMEN_MIN_CONFIDENCE ?? String(DEFAULT_MIN_CONF));
  const enabled = (process.env.LUMEN_ENABLED ?? "true").toLowerCase() !== "false";

  if (!enabled) return;

  // Check connectivity once at startup. Fail silently — Lumen is optional.
  let lumenAvailable = false;
  fetch(`${baseURL}/health`, { signal: AbortSignal.timeout(2000) })
    .then((r) => {
      lumenAvailable = r.ok;
      if (lumenAvailable) {
        pi.log?.("info", `[lumen] connected at ${baseURL}`);
      }
    })
    .catch(() => {
      pi.log?.("warn", `[lumen] server not reachable at ${baseURL} — belief tracking disabled`);
    });

  /**
   * before_model_call: inject the current epistemic state as a system context message.
   * Pi calls this hook before every LLM call, giving us the chance to prepend
   * a belief summary so the model is aware of what Lumen currently believes.
   */
  pi.on("before_model_call", async (_event, ctx) => {
    if (!lumenAvailable) return;

    try {
      const url = `${baseURL}/context?max=${maxBeliefs}&min_confidence=${minConf}`;
      const res = await fetch(url, { signal: AbortSignal.timeout(3000) });
      if (!res.ok) return;

      const text = await res.text();
      if (!text || text.trim() === "No active beliefs in store.") return;

      // Inject as a system-role context message. Pi's transformContext
      // will merge this into the message stream before the LLM sees it.
      ctx.injectContext?.(text);
    } catch {
      // Timeout or network error — skip silently.
    }
  });

  /**
   * after_model_call: extract claims from the assistant's response and
   * send them to Lumen for ingestion into the belief store.
   */
  pi.on("after_model_call", async (event, _ctx) => {
    if (!lumenAvailable) return;

    // Extract text content from the assistant message.
    const text = extractText(event);
    if (!text || text.length < 40) return; // too short to bother

    try {
      await fetch(`${baseURL}/ingest`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ text, min_confidence: minConf }),
        signal: AbortSignal.timeout(5000),
      });
    } catch {
      // Best-effort — never block the agent loop.
    }
  });
}

/** Extract plain text from a model call event's assistant message. */
function extractText(event: unknown): string {
  if (!event || typeof event !== "object") return "";
  const e = event as Record<string, unknown>;

  // Pi emits the full assistant message on after_model_call.
  // The structure follows the AgentMessage format.
  const message = e["message"] ?? e["assistantMessage"];
  if (!message || typeof message !== "object") return "";

  const m = message as Record<string, unknown>;
  const content = m["content"];

  if (typeof content === "string") return content;

  if (Array.isArray(content)) {
    return content
      .filter((c): c is { type: string; text: string } =>
        typeof c === "object" && c !== null && (c as Record<string, unknown>)["type"] === "text"
      )
      .map((c) => c.text)
      .join("\n");
  }

  return "";
}
