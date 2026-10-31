// Pure view logic for the Runtime cards, kept out of the component so the
// edge cases (divide-by-zero, off/empty payloads) are unit-testable.
import type { ProcessStatsResponse, StickyStatsResponse, TranslatorStatsEntry, TranslatorStatsResponse } from './types';
import { fmtInt, fmtPct } from './format';

/** Sticky card is shown only for an enabled payload; {"enabled":false} hides it. */
export function stickyVisible(s: StickyStatsResponse | null | undefined): s is StickyStatsResponse {
  return !!s && s.enabled === true;
}

/** hits/(hits+misses) as a percentage, or null before any lookup (no 0/0 NaN). */
export function stickyHitRate(s: StickyStatsResponse): number | null {
  const hits = s.hits ?? 0;
  const total = hits + (s.misses ?? 0);
  return total > 0 ? (hits / total) * 100 : null;
}

/** Fill of the session table, 0-100, or null when the cap is unknown. */
export function stickyFill(s: StickyStatsResponse): number | null {
  const max = s.max_sessions ?? 0;
  return max > 0 ? ((s.active_sessions ?? 0) / max) * 100 : null;
}

export function stickyHitRateText(s: StickyStatsResponse): string {
  const r = stickyHitRate(s);
  return r === null ? '—' : fmtPct(r);
}

/** Translators card is shown only when at least one translator is registered. */
export function translatorList(t: TranslatorStatsResponse | null | undefined): TranslatorStatsEntry[] {
  return Array.isArray(t?.translators) ? t.translators.filter(translatorHasTraffic) : [];
}

export function fallbackTotal(e: TranslatorStatsEntry): number {
  return (
    (e.fallback_no_compatible_endpoints ?? 0) +
    (e.fallback_translator_does_not_support_passthrough ?? 0) +
    (e.fallback_cannot_passthrough ?? 0)
  );
}

export function fallbackBreakdown(e: TranslatorStatsEntry): string {
  const parts: string[] = [];
  if (e.fallback_no_compatible_endpoints > 0) parts.push(`${fmtInt(e.fallback_no_compatible_endpoints)} no compatible endpoint`);
  if (e.fallback_translator_does_not_support_passthrough > 0)
    parts.push(`${fmtInt(e.fallback_translator_does_not_support_passthrough)} translator lacks passthrough`);
  if (e.fallback_cannot_passthrough > 0) parts.push(`${fmtInt(e.fallback_cannot_passthrough)} cannot passthrough`);
  return parts.length > 0 ? parts.join(' · ') : 'none';
}

/** Rates and latency arrive pre-formatted; show a dash before any traffic. */
export function translatorHasTraffic(e: TranslatorStatsEntry): boolean {
  return (e.total_requests ?? 0) > 0;
}

/** A process payload is usable only with all four groups the card reads, so a
 *  proxy or older build answering with something else shows no card rather
 *  than throwing mid-render. */
export function processUsable(p: ProcessStatsResponse | null | undefined): p is ProcessStatsResponse {
  return !!p && !!p.memory && !!p.garbage_collection && !!p.goroutines && !!p.runtime;
}

// Shape checks run before a payload is cached, so a malformed body is treated
// as a failed poll and the last good snapshot survives. Extra fields are
// ignored; fields the cards format are coerced to a safe default.
const isObj = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null && !Array.isArray(v);
const str = (v: unknown): string => (typeof v === 'string' ? v : '');
const num = (v: unknown): number => (typeof v === 'number' && Number.isFinite(v) ? v : 0);
const optNum = (v: unknown): number | undefined => (typeof v === 'number' && Number.isFinite(v) ? v : undefined);

export function parseSticky(raw: unknown): StickyStatsResponse | null {
  if (!isObj(raw) || typeof raw.enabled !== 'boolean') return null;
  return {
    enabled: raw.enabled,
    active_sessions: optNum(raw.active_sessions),
    insertions: optNum(raw.insertions),
    hits: optNum(raw.hits),
    misses: optNum(raw.misses),
    evictions: optNum(raw.evictions),
    max_sessions: optNum(raw.max_sessions),
    idle_ttl_seconds: optNum(raw.idle_ttl_seconds),
  };
}

export function parseTranslators(raw: unknown): TranslatorStatsResponse | null {
  if (!isObj(raw) || !Array.isArray(raw.translators)) return null;
  const translators: TranslatorStatsEntry[] = [];
  for (const r of raw.translators) {
    if (!isObj(r) || typeof r.translator_name !== 'string' || r.translator_name === '') continue;
    translators.push({
      translator_name: r.translator_name,
      success_rate: str(r.success_rate),
      passthrough_rate: str(r.passthrough_rate),
      average_latency: str(r.average_latency),
      total_requests: num(r.total_requests),
      successful_requests: num(r.successful_requests),
      failed_requests: num(r.failed_requests),
      passthrough_requests: num(r.passthrough_requests),
      translation_requests: num(r.translation_requests),
      streaming_requests: num(r.streaming_requests),
      non_streaming_requests: num(r.non_streaming_requests),
      fallback_no_compatible_endpoints: num(r.fallback_no_compatible_endpoints),
      fallback_translator_does_not_support_passthrough: num(r.fallback_translator_does_not_support_passthrough),
      fallback_cannot_passthrough: num(r.fallback_cannot_passthrough),
    });
  }
  // Rows were sent but none were usable: that is a broken payload, not an empty
  // fleet, so reject it and keep the last good snapshot.
  if (raw.translators.length > 0 && translators.length === 0) return null;
  return { timestamp: str(raw.timestamp), translators, summary: {} as TranslatorStatsResponse['summary'] };
}

export function parseProcess(raw: unknown): ProcessStatsResponse | null {
  if (!isObj(raw) || !isObj(raw.memory) || !isObj(raw.garbage_collection) || !isObj(raw.goroutines) || !isObj(raw.runtime)) {
    return null;
  }
  const { memory: m, garbage_collection: g, goroutines: r, runtime: rt } = raw;
  return {
    timestamp: str(raw.timestamp),
    memory: {
      heap_alloc: str(m.heap_alloc),
      heap_sys: str(m.heap_sys),
      heap_inuse: str(m.heap_inuse),
      memory_pressure: str(m.memory_pressure),
    },
    garbage_collection: { avg_gc_pause: str(g.avg_gc_pause), num_gc_cycles: num(g.num_gc_cycles) },
    goroutines: { health_status: str(r.health_status), count: num(r.count) },
    runtime: { uptime: str(rt.uptime), go_version: str(rt.go_version), num_cpu: num(rt.num_cpu) },
  };
}
