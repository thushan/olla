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
  return Array.isArray(t?.translators) ? t.translators : [];
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
