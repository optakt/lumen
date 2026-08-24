/**
 * Lumen Pi Extension
 *
 * Integrates the Lumen belief store with the Pi agent harness.
 *
 * Three integration layers:
 *
 * 1. Context injection — before each model call, prepends the current epistemic
 *    state (beliefs + self-model commitments) into the context window.
 *
 * 2. Automatic extraction — after each assistant turn, the response text is
 *    sent to Lumen's NLP pipeline for claim extraction and storage.
 *
 * 3. Explicit tools — three agent tools for deliberate epistemic tracking:
 *    - `lumen_assert`: assert a belief with kind, confidence, and provenance
 *    - `lumen_query`: query the belief store by predicate
 *    - `lumen_bio`: retrieve the full epistemic biography of a belief
 *
 * Installation:
 *   cp lumen.ts ~/.pi/agent/extensions/lumen.ts
 *
 * Prerequisites:
 *   lumen-server -addr :3737 -db ~/.lumen/beliefs.db
 *
 * Configuration (environment variables):
 *   LUMEN_URL              Lumen server base URL (default: http://localhost:3737)
 *   LUMEN_MAX_BELIEFS      Max beliefs to inject per turn (default: 8)
 *   LUMEN_MIN_CONFIDENCE   Min confidence threshold 0–1 (default: 0.5)
 *   LUMEN_SELF_CONTEXT     Include self-model section in context (default: true)
 *   LUMEN_ENABLED          Set to "false" to disable (default: true)
 */

import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

const DEFAULT_URL      = "http://localhost:3737";
const DEFAULT_MAX      = 8;
const DEFAULT_MIN_CONF = 0.5;

export default function (pi: ExtensionAPI) {
  const baseURL    = (process.env.LUMEN_URL ?? DEFAULT_URL).replace(/\/$/, "");
  const maxBeliefs = parseInt(process.env.LUMEN_MAX_BELIEFS ?? String(DEFAULT_MAX), 10);
  const minConf    = parseFloat(process.env.LUMEN_MIN_CONFIDENCE ?? String(DEFAULT_MIN_CONF));
  const selfCtx    = (process.env.LUMEN_SELF_CONTEXT ?? "true").toLowerCase() !== "false";
  const enabled    = (process.env.LUMEN_ENABLED ?? "true").toLowerCase() !== "false";

  if (!enabled) return;

  let lumenAvailable = false;
  fetch(`${baseURL}/v1/health`, { signal: AbortSignal.timeout(2000) })
    .then((r) => {
      lumenAvailable = r.ok;
      if (lumenAvailable) pi.log?.("info", `[lumen] connected at ${baseURL}`);
    })
    .catch(() => {
      pi.log?.("warn", `[lumen] not reachable at ${baseURL} — belief tracking disabled`);
    });

  // ─── Tool: lumen_assert ──────────────────────────────────────────────────
  // Assert an explicit epistemic commitment. Returns the belief ID for use
  // in lumen_bio or as a source in future assertions.

  pi.registerTool({
    name: "lumen_assert",
    description:
      "Assert an explicit epistemic commitment into the belief store. " +
      "Use this when making a claim you want tracked — a decision, a conclusion, " +
      "or a position that might later need to be revised. " +
      "Returns the belief ID.",
    parameters: {
      type: "object",
      required: ["content"],
      properties: {
        content: {
          type: "string",
          description: "The claim to assert.",
        },
        kind: {
          type: "string",
          enum: ["asserted", "derived", "retrieved", "corrected"],
          description: "Epistemic kind. Default: asserted.",
        },
        confidence: {
          type: "number",
          description: "Confidence in [0,1]. Default: 0.75.",
        },
        sources: {
          type: "array",
          items: { type: "string" },
          description: "Belief IDs this claim derives from.",
        },
      },
    },
    async execute(_id, params) {
      if (!lumenAvailable) {
        return { content: [{ type: "text", text: "Lumen not available." }] };
      }
      const body = {
        kind:       params.kind ?? "asserted",
        content:    params.content,
        confidence: params.confidence,
        sources:    params.sources,
      };
      try {
        const res = await fetch(`${baseURL}/v1/self/claim`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body),
          signal: AbortSignal.timeout(5000),
        });
        if (!res.ok) {
          return { content: [{ type: "text", text: `Assert failed: ${res.status}` }] };
        }
        const data = await res.json() as { id: string; frame: string };
        return {
          content: [{
            type: "text",
            text: `Asserted [${data.frame}] "${params.content}" → id: ${data.id}`,
          }],
        };
      } catch (err) {
        return { content: [{ type: "text", text: `Assert error: ${String(err)}` }] };
      }
    },
  });

  // ─── Tool: lumen_query ──────────────────────────────────────────────────
  // Query the belief store.

  pi.registerTool({
    name: "lumen_query",
    description:
      "Query active beliefs from the store. Returns beliefs filtered by confidence " +
      "and optionally formatted as JSON for programmatic use.",
    parameters: {
      type: "object",
      properties: {
        min_confidence: {
          type: "number",
          description: "Minimum confidence (0–1). Default: 0.0 (all beliefs).",
        },
        format: {
          type: "string",
          enum: ["text", "json"],
          description: "Output format. Default: text.",
        },
        max: {
          type: "number",
          description: "Maximum beliefs to return. Default: 20.",
        },
      },
    },
    async execute(_id, params) {
      if (!lumenAvailable) {
        return { content: [{ type: "text", text: "Lumen not available." }] };
      }
      const min  = params.min_confidence ?? 0;
      const fmt  = params.format ?? "text";
      const max  = params.max ?? 20;
      try {
        const res = await fetch(
          `${baseURL}/v1/context?min_confidence=${min}&max=${max}&format=${fmt}`,
          { signal: AbortSignal.timeout(5000) },
        );
        const text = await res.text();
        return { content: [{ type: "text", text }] };
      } catch (err) {
        return { content: [{ type: "text", text: `Query error: ${String(err)}` }] };
      }
    },
  });

  // ─── Tool: lumen_bio ────────────────────────────────────────────────────
  // Retrieve the epistemic biography of a belief — its full arc of confidence
  // changes, revisions, and retractions over time.

  pi.registerTool({
    name: "lumen_bio",
    description:
      "Retrieve the epistemic biography of a belief: every confidence change, " +
      "revision, and retraction with timestamps and causes. " +
      "Useful for understanding why a belief changed and what caused each shift. " +
      "Pass the belief ID from lumen_assert.",
    parameters: {
      type: "object",
      required: ["id"],
      properties: {
        id: {
          type: "string",
          description: "Belief ID (returned by lumen_assert).",
        },
        threshold: {
          type: "number",
          description: "Minimum confidence change to include in the arc. Default: 0.05.",
        },
      },
    },
    async execute(_id, params) {
      if (!lumenAvailable) {
        return { content: [{ type: "text", text: "Lumen not available." }] };
      }
      const threshold = params.threshold ?? 0.05;
      try {
        const res = await fetch(
          `${baseURL}/v1/self/biography/${encodeURIComponent(params.id)}?threshold=${threshold}`,
          { signal: AbortSignal.timeout(5000) },
        );
        const text = await res.text();
        return { content: [{ type: "text", text }] };
      } catch (err) {
        return { content: [{ type: "text", text: `Biography error: ${String(err)}` }] };
      }
    },
  });


  // ─── Tool: lumen_correct ────────────────────────────────────────────────
  // Explicitly correct a prior belief: retracts the old claim (marking it
  // suspect via sentinel cascade) and asserts the replacement.

  pi.registerTool({
    name: "lumen_correct",
    description:
      "Record a correction: retract a prior belief and assert its replacement. " +
      "The retracted belief is marked suspect (visible with ⚠) so the correction " +
      "is traceable. Use when you have changed your mind about a prior claim.",
    parameters: {
      type: "object",
      required: ["replaces_id", "content"],
      properties: {
        replaces_id: {
          type: "string",
          description: "ID of the belief being corrected (from lumen_assert).",
        },
        content: {
          type: "string",
          description: "The corrected claim.",
        },
        confidence: {
          type: "number",
          description: "Confidence in the corrected claim. Default: 0.75.",
        },
        reason: {
          type: "string",
          description: "Why the prior claim was wrong.",
        },
      },
    },
    async execute(_id, params) {
      if (!lumenAvailable) {
        return { content: [{ type: "text", text: "Lumen not available." }] };
      }
      try {
        const res = await fetch(`${baseURL}/v1/self/correct`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            replaces_id: params.replaces_id,
            content:     params.content,
            confidence:  params.confidence,
            reason:      params.reason,
          }),
          signal: AbortSignal.timeout(5000),
        });
        if (!res.ok) {
          return { content: [{ type: "text", text: `Correct failed: ${res.status}` }] };
        }
        const data = await res.json() as { retracted_id: string; new_id: string };
        return {
          content: [{
            type: "text",
            text: `Corrected: retracted ${data.retracted_id} → new claim ${data.new_id}`,
          }],
        };
      } catch (err) {
        return { content: [{ type: "text", text: `Correct error: ${String(err)}` }] };
      }
    },
  });

  // ─── Lifecycle hooks ────────────────────────────────────────────────────

  pi.on("before_model_call", async (_event, ctx) => {
    if (!lumenAvailable) return;
    try {
      const parts: string[] = [];

      const gr = await fetch(
        `${baseURL}/v1/context?max=${maxBeliefs}&min_confidence=${minConf}`,
        { signal: AbortSignal.timeout(3000) },
      );
      if (gr.ok) {
        const text = await gr.text();
        if (text && text.trim() !== "No active beliefs in store.") parts.push(text);
      }

      if (selfCtx) {
        const sr = await fetch(`${baseURL}/v1/self/context`, { signal: AbortSignal.timeout(3000) });
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
      await fetch(`${baseURL}/v1/ingest`, {
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
