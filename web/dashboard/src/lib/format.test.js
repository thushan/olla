import { describe, it, expect } from 'vitest';
import {
  fmtBytes,
  fmtPct,
  fmtMs,
  fmtInt,
  scalePct,
  fmtDuration,
  fmtAgo,
  fmtUntil,
  fmtUptime,
} from './format.js';

// Fixed reference instant for the time tests; iso values below are computed
// relative to this so the assertions don't depend on wall clock.
const NOW = Date.UTC(2026, 6, 27, 12, 0, 0);
const iso = (offsetMs) => new Date(NOW - offsetMs).toISOString();
const isoFuture = (offsetMs) => new Date(NOW + offsetMs).toISOString();

// Byte values cross-checked against pkg/format.Bytes (see pkg/format/format.go).
// Drift here means the UI disagrees with the API on the same number, so this
// table is the contract.
const BYTE_CASES = [
  [0, '0 B'],
  [1, '1 B'],
  [999, '999 B'],
  [1000, '1000 B'],
  [1023, '1023 B'],
  [1024, '1.00 KB'],
  [1536, '1.50 KB'],
  [1048576, '1.00 MB'],
  [1572864, '1.50 MB'],
  [1073741824, '1.00 GB'],
  [1099511627776, '1.00 TB'],
  [1125899906842624, '1.00 PB'],
  // Real fixture from the spec: a model SizeBytes of 5_000_000_000.
  [5000000000, '4.66 GB'],
  // Mockup's "184GB" traffic figure for sanity (Go and JS agree on 172.05).
  [184738291200, '172.05 GB'],
];

describe('fmtBytes', () => {
  for (const [input, expected] of BYTE_CASES) {
    it(`maps ${input} -> "${expected}"`, () => {
      expect(fmtBytes(input)).toBe(expected);
    });
  }

  it('treats non-finite / negative as zero', () => {
    expect(fmtBytes(NaN)).toBe('0 B');
    expect(fmtBytes(-1)).toBe('0 B');
    expect(fmtBytes(undefined)).toBe('0 B');
  });
});

describe('fmtPct', () => {
  it('matches pkg/format.Percentage for boundaries and one-decimal values', () => {
    expect(fmtPct(0)).toBe('0%');
    expect(fmtPct(100)).toBe('100%');
    expect(fmtPct(97.4)).toBe('97.4%');
    expect(fmtPct(99.95)).toBe('100.0%');
  });
});

describe('fmtMs', () => {
  it('renders sub-second and second-scale latencies', () => {
    expect(fmtMs(0)).toBe('0ms');
    expect(fmtMs(288)).toBe('288ms');
    expect(fmtMs(1841)).toBe('1.8s');
  });
});

describe('fmtInt', () => {
  it('groups with en-AU thousands separators', () => {
    expect(fmtInt(1337)).toBe('1,337');
    expect(fmtInt(1284431)).toBe('1,284,431');
    expect(fmtInt(0)).toBe('0');
    expect(fmtInt(null)).toBe('0');
  });
});

describe('scalePct', () => {
  it('sqrt-scales so small values stay visible against a large max', () => {
    expect(scalePct(0, 100)).toBe(0);
    expect(scalePct(100, 100)).toBe(100);
    // 288 against a 29400 max should be ~9.9%, not the ~0.98% a linear scale
    // would give: that's the whole point of sqrt scaling.
    expect(scalePct(288, 29400)).toBeCloseTo(9.9, 1);
  });

  it('clamps above 100', () => {
    expect(scalePct(200, 100)).toBe(100);
  });
});

describe('fmtDuration', () => {
  // Mirrors pkg/format.TimeDuration: single-digit for <10s, then m/h/d.
  it('matches Go TimeDuration at the boundaries Go cares about', () => {
    expect(fmtDuration(0)).toBe('0s');
    expect(fmtDuration(5_000)).toBe('5s');
    expect(fmtDuration(9_000)).toBe('9s');
    expect(fmtDuration(45_000)).toBe('45s');
    expect(fmtDuration(60_000)).toBe('1m');
    expect(fmtDuration(5 * 60_000)).toBe('5m');
    expect(fmtDuration(2 * 3600_000)).toBe('2h');
    expect(fmtDuration(3 * 86400_000)).toBe('3d');
  });
});

describe('fmtAgo', () => {
  it('returns empty for missing/zero so callers can show "never"', () => {
    expect(fmtAgo(null)).toBe('');
    expect(fmtAgo('')).toBe('');
    expect(fmtAgo('not-a-date')).toBe('');
  });

  it('renders compact "<dur> ago" relative to the supplied now', () => {
    expect(fmtAgo(iso(4_000), NOW)).toBe('4s ago');
    expect(fmtAgo(iso(2 * 60_000), NOW)).toBe('2m ago');
    expect(fmtAgo(iso(3 * 3600_000), NOW)).toBe('3h ago');
    expect(fmtAgo(iso(2 * 86400_000), NOW)).toBe('2d ago');
  });
});

describe('fmtUntil', () => {
  it('returns empty for missing, "now" for past', () => {
    expect(fmtUntil(null)).toBe('');
    expect(fmtUntil(iso(5_000), NOW)).toBe('now');
  });

  it('renders "in <dur>" for future instants', () => {
    expect(fmtUntil(isoFuture(30_000), NOW)).toBe('in 30s');
    expect(fmtUntil(isoFuture(5 * 60_000), NOW)).toBe('in 5m');
  });
});

describe('fmtUptime', () => {
  it('mirrors pkg/format.Duration2 across the day boundary', () => {
    expect(fmtUptime(iso(45_000), NOW)).toBe('45s');
    expect(fmtUptime(iso(20 * 60_000), NOW)).toBe('20m');
    expect(fmtUptime(iso(2 * 3600_000 + 15 * 60_000), NOW)).toBe('2h 15m');
    expect(fmtUptime(iso(3 * 86400_000 + 4 * 3600_000), NOW)).toBe('3d 4h');
  });

  it('returns empty when start_time missing', () => {
    expect(fmtUptime(null)).toBe('');
  });
});
