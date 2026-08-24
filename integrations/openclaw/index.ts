/**
 * Lumen OpenClaw Plugin
 *
 * Integrates the Lumen belief store (https://github.com/optakt/lumen) with
 * OpenClaw. Two hooks wire into the agent loop:
 *
 *   agent_turn_prepare — fetches the current epistemic state from Lumen and
 *     prepends it to the context before each agent turn. Both the general
 *     belief context and the self-model commitments are injected.
 *
 *   llm_output — extracts claims from the assistant's response via Lumen's
 *     NLP extraction pipeline and asserts them into the belief store.
 *
 * The plugin degrades gracefully: if Lumen is unreachable, both hooks skip
 * silently and OpenClaw runs as if the plugin were not installed.
 *
 * Prerequisites:
 *   Start the Lumen server before launching OpenClaw:
 *     lumen-server -addr :3737 -db ~/.lumen/beliefs.db
 *
 * Configuration (openclaw.config.json):
 *   {
 *     "plugins": {
 *       "lumen": {
 *         "url": "http://localhost:3737",
 *         "maxBeliefs": 8,
 *         "minConfidence": 0.5,
 *         "selfContext": true
 *       }
 *     }
 *   }
 */

import { definePluginEntry } from "openclaw/plugin-sdk/plugin-entry";

const DEFAULT_URL       = "http://localhost:3737";
const DEFAULT_MAX       = 8;
const DEFAULT_MIN_CONF  = 0.5;

interface LumenConfig {
  url?: string;
  maxBeliefs?: number;
  minConfidence?: number;
  selfContext?: boolean;
}

export default definePluginEntry({
  id: "lumen",
  name: "Lumen Belief Store",
  description:
    "Tracks epistemic state across agent turns. " +
    "Injects current beliefs before each model call; " +
    "extracts and asserts new claims from assistant responses.",

  register(api, config: LumenConfig = {}) {
    const baseURL    = (config.url ?? DEFAULT_URL).replace(/\/$/, "");
    const maxBeliefs = config.maxBeliefs  ?? DEFAULT_MAX;
    const minConf    = config.minConfidence ?? DEFAULT_MIN_CONF;
    const selfCtx    = config.selfContext ?? true;

    // Check connectivity once at startup; disable both hooks if unreachable.
    let available = false;
    fetch(`${baseURL}/v1/health`, { signal: AbortSignal.timeout(2000) })
      .then((r) => { available = r.ok; })
      .catch(() => {
        api.log?.warn(`[lumen] server not reachable at ${baseURL} — belief tracking disabled`);
      });

    // ─── agent_turn_prepare ───────────────────────────────────────────────
    // Fires before each agent turn. Return prependContext to inject text
    // at the top of the context window before the model call.

    api.registerHook("agent_turn_prepare", async () => {
      if (!available) return undefined;

      try {
        const parts: string[] = [];

        const gr = await fetch(
          `${baseURL}/v1/context?max=${maxBeliefs}&min_confidence=${minConf}`,
          { signal: AbortSignal.timeout(3000) },
        );
        if (gr.ok) {
          const text = await gr.text();
          if (text && text.trim() !== "No active beliefs in store.") {
            parts.push(text.trim());
          }
        }

        if (selfCtx) {
          const sr = await fetch(
            `${baseURL}/v1/self/context`,
            { signal: AbortSignal.timeout(3000) },
          );
          if (sr.ok) {
            const text = await sr.text();
            if (text && text.trim() !== "No active self-model claims.") {
              parts.push(text.trim());
            }
          }
        }

        if (parts.length === 0) return undefined;
        return { prependContext: parts.join("\n\n") };
      } catch {
        return undefined;
      }
    });

    // ─── llm_output ───────────────────────────────────────────────────────
    // Fires after each LLM call. assistantTexts contains the full text
    // blocks from the assistant response — ideal for claim extraction.

    api.registerHook("llm_output", async (event) => {
      if (!available) return;

      const texts = event.assistantTexts?.filter((t) => t && t.length >= 40) ?? [];
      if (texts.length === 0) return;

      const combined = texts.join("\n\n");
      try {
        await fetch(`${baseURL}/v1/ingest`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ text: combined, min_confidence: minConf }),
          signal: AbortSignal.timeout(5000),
        });
      } catch {
        // Best-effort — never block or throw in a hook.
      }
    });
  },
});
