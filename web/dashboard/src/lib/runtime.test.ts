import { describe, it, expect } from 'vitest';
import {
  stickyVisible,
  stickyHitRate,
  stickyHitRateText,
  stickyFill,
  translatorList,
  processUsable,
  translatorHasTraffic,
  parseSticky,
  parseTranslators,
  parseProcess,
  fallbackTotal,
  fallbackBreakdown,
} from './runtime';
import type { TranslatorStatsEntry } from './types';

const entry = (o: Partial<TranslatorStatsEntry> = {}): TranslatorStatsEntry => ({
  translator_name: 'anthropic',
  success_rate: '100%',
  passthrough_rate: '0%',
  average_latency: '5ms',
  total_requests: 0,
  successful_requests: 0,
  failed_requests: 0,
  passthrough_requests: 0,
  translation_requests: 0,
  streaming_requests: 0,
  non_streaming_requests: 0,
  fallback_no_compatible_endpoints: 0,
  fallback_translator_does_not_support_passthrough: 0,
  fallback_cannot_passthrough: 0,
  ...o,
});

describe('sticky helpers', () => {
  it('hides the card for {"enabled":false} and for no payload', () => {
    expect(stickyVisible({ enabled: false })).toBe(false);
    expect(stickyVisible(null)).toBe(false);
    expect(stickyVisible({ enabled: true })).toBe(true);
  });
  it('computes hit rate without dividing by zero', () => {
    expect(stickyHitRate({ enabled: true, hits: 0, misses: 0 })).toBeNull();
    expect(stickyHitRateText({ enabled: true })).toBe('—');
    expect(stickyHitRate({ enabled: true, hits: 39, misses: 1 })).toBeCloseTo(97.5);
    expect(stickyHitRateText({ enabled: true, hits: 3, misses: 1 })).toBe('75.0%');
  });
  it('computes fill and tolerates a missing cap', () => {
    expect(stickyFill({ enabled: true, active_sessions: 5, max_sessions: 1000 })).toBeCloseTo(0.5);
    expect(stickyFill({ enabled: true, active_sessions: 5, max_sessions: 0 })).toBeNull();
  });
});

describe('translator helpers', () => {
  it('treats an empty or missing list as no translators', () => {
    expect(translatorList(null)).toEqual([]);
    expect(translatorList({ translators: [] } as never)).toEqual([]);
  });
  it('sums and describes fallbacks', () => {
    const e = entry({ fallback_no_compatible_endpoints: 2, fallback_cannot_passthrough: 1 });
    expect(fallbackTotal(e)).toBe(3);
    expect(fallbackBreakdown(e)).toBe('2 no compatible endpoint · 1 cannot passthrough');
    expect(fallbackBreakdown(entry())).toBe('none');
  });
});

describe('processUsable', () => {
  it('rejects payloads missing a group the card reads', () => {
    expect(processUsable(null)).toBe(false);
    expect(processUsable({ system: {} } as never)).toBe(false);
  });
});

describe('translatorHasTraffic', () => {
  it('is false at zero requests and true once any request has been counted', () => {
    expect(translatorHasTraffic(entry())).toBe(false);
    expect(translatorHasTraffic(entry({ total_requests: 1 }))).toBe(true);
  });
});

describe('payload parsers', () => {
  it('rejects bodies missing what the card needs', () => {
    expect(parseSticky({})).toBeNull();
    expect(parseTranslators({})).toBeNull();
    expect(parseProcess({})).toBeNull();
  });
  it('drops invalid translator rows and tolerates extra fields', () => {
    const r = parseTranslators({ extra: 1, translators: [null, 3, { translator_name: '' }, { translator_name: 'a', total_requests: 2, x: 1 }] });
    expect(r?.translators.map((t) => t.translator_name)).toEqual(['a']);
  });
  it('rejects a non-empty translator list with no usable rows, but accepts an empty one', () => {
    expect(parseTranslators({ translators: [null] })).toBeNull();
    expect(parseTranslators({ translators: [] })?.translators).toEqual([]);
  });
  it('coerces non-string process fields so formatting cannot throw', () => {
    const p = parseProcess({ memory: { memory_pressure: 7 }, garbage_collection: {}, goroutines: {}, runtime: {} });
    expect(p?.memory.memory_pressure).toBe('');
  });
});
