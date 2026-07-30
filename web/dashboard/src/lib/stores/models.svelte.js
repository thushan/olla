// /internal/status/models poller. Drives ModelsPanel. Detailed/grouped form
// is requested so the panel can render family groups without client re-group.
//
// Per-model request/latency stats come from a separate endpoint
// (/internal/stats/models) which is NOT ETag-wrapped. We fetch both on the
// same 15s cadence and merge stats by model name into `stats`/`stats_summary`
// on the store payload, so the panel reads a single object.
import { createPollStore } from './poll-store.svelte.js';

const MODELS_INTERVAL_MS = 15000;
// detailed=true (not =1): the handler compares against the literal "true".
// group=family populates model_groups so the panel can render families.
const STATUS_URL = '/internal/status/models?detailed=true&group=family';
const STATS_URL = '/internal/stats/models?include_summary=true';

export const models = createPollStore({
  name: 'models',
  url: STATUS_URL,
  intervalMs: MODELS_INTERVAL_MS,
  // Companion fetch merged into the primary payload after each successful
  // poll. Kept here (not a second store) so the single-scheduler invariant
  // holds: one job, one tick, one merged payload.
  companionUrl: STATS_URL,
  companionMerge: mergeStats,
});

/**
 * Merge /internal/stats/models into the status/models payload. Per-model
 * stats are keyed by name in `stats`; the summary is exposed verbatim.
 * Failure of the companion fetch is non-fatal: the panel renders the
 * status fields without the stats columns rather than blanking the panel.
 *
 * The join is case-insensitive: the routing path lowercases model names
 * (so /internal/stats/models reports "qwen3:8b") while discovery preserves
 * the backend's original casing ("qwen3:8B"). We index stats by lowercased
 * name and plant an alias under each name exactly as the status payload
 * reports it, so the panel's `stats[m.name]` lookup resolves regardless of
 * which side changed case.
 */
function mergeStats(payload, companion) {
  if (!companion || typeof companion !== 'object') {
    return { ...payload, stats: {}, stats_summary: null };
  }
  const byLower = {};
  for (const m of companion.models ?? []) {
    if (m && m.name) byLower[String(m.name).toLowerCase()] = m;
  }
  // Seed with the lowercased keys so direct lowercase lookups still work,
  // then alias every name the status payload reports onto its match.
  const byName = { ...byLower };
  for (const name of discoveryModelNames(payload)) {
    const hit = byLower[name.toLowerCase()];
    // Re-key the entry's name to the discovery casing so it does not clobber
    // the panel's original-case model name when spread onto the row.
    if (hit) byName[name] = { ...hit, name };
  }
  return {
    ...payload,
    stats: byName,
    stats_summary: companion.summary ?? null,
  };
}

// Collect every model name the status payload exposes, across the grouped
// and flat shapes the handler may return, so the join aliases them all.
function discoveryModelNames(payload) {
  if (!payload || typeof payload !== 'object') return [];
  const names = new Set();
  const push = (n) => { if (n) names.add(String(n)); };
  for (const m of payload.recent_models ?? []) push(m?.name);
  for (const group of payload.model_groups ?? []) {
    for (const m of group?.models ?? []) push(m?.name);
  }
  const byFamily = payload.models_by_family ?? {};
  if (byFamily && typeof byFamily === 'object') {
    for (const list of Object.values(byFamily)) {
      for (const entry of list ?? []) push(typeof entry === 'string' ? entry : entry?.name);
    }
  }
  return names;
}
