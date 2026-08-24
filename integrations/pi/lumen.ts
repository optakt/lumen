/**
 * Lumen Pi Extension
 *
 * Integrates the Lumen belief store with the Pi agent harness.
 *
 * Two integration layers:
 *
 * 1. General belief tracking — before each model call, injects the current
 *    epistemic state from Lumen. After each assistant turn, ingests the
 *    response text for automatic claim extraction.
 *
 * 2. Self-model — provides `pi.lumen.assert()` and `pi.lumen.correct()` for
 *    tools and skill files that want to explicitly record epistemic commitments
 *    with full provenance (kind, confidence, sources).
 *
 * Installation:
 *   cp lumen.ts ~/.pi/agent/extensions/lumen.ts
 *
 * Configuration (environment variables):
 *   LUMEN_URL              Lumen server base URL (default: http://localhost:3737)
 *   LUMEN_MAX_BELIEFS      Max beliefs to inject (default: 8)
 *   LUMEN_MIN_CONFIDENCE   Min confidence threshold 0–1 (default: 0.5)
 *   LUMEN_SELF_CONTEXT     Include self-model section in injected context (default: true)
 *   LUMEN_ENABLED          Set to "false" to disable
 */

import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

const DEFAULT_URL      = "http://localhost:3737";
const DEFAULT_MAX      = 8;
const DEFAULT_MIN_CONF = 0.5;

export type ClaimKind = "asserted" | "derived" | "retrieved" | "corrected";

export interface LumenAPI {
  /** Assert an explicit epistemic claim into the self-model. */
  assert(content: string, options?: {
    kind?: ClaimKind;
    confidence?: number;
    frame?: string;
    sources?: string[];
    tags?: string[];
  }): Promise<string | null>; // returns belief ID or null on failure

  /** Record a correction: retract a prior claim and assert its replacement. */
  correct(replacesID: string, newContent: string, options?: {
    confidence?: number;
    reason?: string;
  }): Promise<{ retractedId: string; newId: string } | null>;
}

export default function (pi: ExtensionAPI) {
  const baseURL    = (process.env.LUMEN_URL ?? DEFAULT_URL).replace(/\/$/, "");
  const maxBeliefs = parseInt(process.env.LUMEN_MAX_BELIEFS ?? String(DEFAULT_MAX), 10);
  const minConf    = parseFloat(process.env.LUMEN_MIN_CONFIDENCE ?? String(DEFAULT_MIN_CONF));
  const selfCtx    = (process.env.LUMEN_SELF_CONTEXT ?? "true").toLowerCase() !== "false";
  const enabled    = (process.env.LUMEN_ENABLED ?? "true").toLowerCase() !== "false";

  if (!enabled) return;

  let lumenAvailable = false;
  fetch(`${baseURL}/health`, { signal: AbortSignal.timeout(2000) })
    .then((r) => {
      lumenAvailable = r.ok;
      if (lumenAvailable) pi.log?.("info", `[lumen] connected at ${baseURL}`);
    })
    .catch(() => {
      pi.log?.("warn", `[lumen] not reachable at ${baseURL} — belief tracking disabled`);
    });

  // ─── Programmatic API ───────────────────────────────────────────────────────

  const lumenAPI: LumenAPI = {
    async assert(content, opts = {}) {
      if (!lumenAvailable) return null;
      try {
        const res = await fetch(`${baseURL}/self/claim`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            kind:       opts.kind ?? "asserted",
            content,
            confidence: opts.confidence,
            frame:      opts.frame,
            sources:    opts.sources,
            tags:       opts.tags,
          }),
          signal: AbortSignal.timeout(5000),
        });
        if (!res.ok) return null;
        const data = await res.json() as { id: string };
        return data.id ?? null;
      } catch { return null; }
    },

    async correct(replacesID, newContent, opts = {}) {
      if (!lumenAvailable) return null;
      try {
        const res = await fetch(`${baseURL}/self/correct`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            replaces_id: replacesID,
            content:     newContent,
            confidence:  opts.confidence,
            reason:      opts.reason,
          }),
          signal: AbortSignal.timeout(5000),
        });
        if (!res.ok) return null;
        const data = await res.json() as { retracted_id: string; new_id: string };
        return { retractedId: data.retracted_id, newId: data.new_id };
      } catch { return null; }
    },
  };

  // Expose on pi so tools and skills can import it.
  (pi as unknown as Record<string, unknown>)["lumen"] = lumenAPI;

  // ─── Lifecycle hooks ────────────────────────────────────────────────────────

  pi.on("before_model_call", async (_event, ctx) => {
    if (!lumenAvailable) return;
    try {
      const parts: string[] = [];

      // General belief context.
      const gr = await fetch(
        `${baseURL}/context?max=${maxBeliefs}&min_confidence=${minConf}`,
        { signal: AbortSignal.timeout(3000) }
      );
      if (gr.ok) {
        const text = await gr.text();
        if (text && text.trim() !== "No active beliefs in store.") parts.push(text);
      }

      // Self-model context.
      if (selfCtx) {
        const sr = await fetch(`${baseURL}/self/context`, { signal: AbortSignal.timeout(3000) });
        if (sr.ok) {
          const text = await sr.text();
          if (text && text.trim() !== "No active self-model claims.") parts.push(text);
        }
      }

      if (parts.length > 0) ctx.injectContext?.(parts.join("\n\n"));
    } catch { /* fail open */ }
  });

  pi.on("after_model_call", async (event, _ctx) => {
    if (!lumenAvailable) return;
    const text = extractText(event);
    if (!text || text.length < 40) return;
    try {
      await fetch(`${baseURL}/ingest`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ text, min_confidence: minConf }),
        signal: AbortSignal.timeout(5000),
      });
    } catch { /* best-effort */ }
  });
}

function extractText(event: unknown): string {
  if (!event || typeof event !== "object") return "";
  const e = event as Record<string, unknown>;
  const message = e["message"] ?? e["assistantMessage"];
  if (!message || typeof message !== "object") return "";
  const content = (message as Record<string, unknown>)["content"];
  if (typeof content === "string") return content;
  if (Array.isArray(content)) {
    return content
      .filter((c): c is { type: string; text: string } =>
        typeof c === "object" && c !== null &&
        (c as Record<string, unknown>)["type"] === "text"
      )
      .map((c) => c.text)
      .join("\n");
  }
  return "";
}
