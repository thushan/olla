import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import type { StatusResponse } from '../types';

// Pure helpers drive both the strip's math and its restart/gap guards; the
// ring buffer just wires them into append(). Cover the helpers in isolation
// first, then exercise append() for the delta/reset/eviction paths.

import {
  history,
  MAX_SAMPLES,
  clampNonNegative,
  computeRate,
  detectRestart,
  parseTrafficBytes,
} from './history.svelte';

function makeStatus(opts: {
  start_time?: string;
  total_requests?: number;
  total_traffic?: string;
  active_connections?: number;
}): StatusResponse {
  return {
    timestamp: '2026-01-01T00:00:00Z',
    proxy: { engine: 'olla', profile: 'ollama', balancer: 'recommended' },
    endpoints: [],
    security: { status: 'ok', blocked_ips: 0, violations: { rate_limits: 0, size_limits: 0 } },
    system: {
      start_time: opts.start_time ?? '2026-01-01T00:00:00Z',
      status: 'healthy',
      endpoints_up: '0/0',
      success_rate: '0%',
      avg_latency: '0ms',
      total_traffic: opts.total_traffic ?? '0 B',
      uptime: '0s',
      version: 'v0.0.0',
      commit: 'deadbeef',
      active_connections: opts.active_connections ?? 0,
      security_violations: 0,
      total_requests: opts.total_requests ?? 0,
      total_failures: 0,
    },
  };
}

beforeEach(() => {
  history.reset();
});

afterEach(() => {
  vi.useRealTimers();
});

describe('pure helpers', () => {
  it('clampNonNegative passes through positives and zeroes negatives', () => {
    expect(clampNonNegative(5)).toBe(5);
    expect(clampNonNegative(0)).toBe(0);
    expect(clampNonNegative(-3)).toBe(0);
    expect(clampNonNegative(Number.NaN)).toBe(0);
  });

  it('computeRate scales a delta to per-second and clamps negative input', () => {
    expect(computeRate(10, 5000)).toBe(2); // 10 over 5 s
    expect(computeRate(0, 5000)).toBe(0);
    expect(computeRate(-5, 5000)).toBe(0); // counter reset
    expect(computeRate(10, 0)).toBe(0); // divide-by-zero guard
  });

  it('detectRestart only fires on a real start_time change', () => {
    expect(detectRestart(null, '2026-01-01')).toBe(false); // first sample
    expect(detectRestart('2026-01-01', '2026-01-01')).toBe(false);
    expect(detectRestart('2026-01-01', '2026-01-02')).toBe(true);
  });

  it('parseTrafficBytes recovers raw bytes from the formatted string', () => {
    expect(parseTrafficBytes(undefined)).toBe(0);
    expect(parseTrafficBytes('0 B')).toBe(0);
    expect(parseTrafficBytes('1.00 KB')).toBe(1024);
    expect(parseTrafficBytes('172.02 GB')).toBeCloseTo(172.02 * 1024 ** 3, -2);
  });
});

describe('history.append delta math', () => {
  it('seeds a zero-delta baseline on the first sample', () => {
    vi.useFakeTimers();
    vi.setSystemTime(1_000);
    history.append(makeStatus({ total_requests: 100 }));
    expect(history.length).toBe(1);
    expect(history.samples[0].reqPerSec).toBe(0);
    expect(history.samples[0].bytesPerSec).toBe(0);
  });

  it('computes a positive rate from a monotonic counter delta', () => {
    vi.useFakeTimers();
    vi.setSystemTime(1_000);
    history.append(makeStatus({ total_requests: 100 }));
    vi.setSystemTime(6_000); // 5 s later
    history.append(makeStatus({ total_requests: 110 }));
    expect(history.samples[1].reqPerSec).toBe(2); // 10 over 5 s
  });

  it('produces a zero delta when the body is identical (304 path)', () => {
    vi.useFakeTimers();
    vi.setSystemTime(1_000);
    history.append(makeStatus({ total_requests: 100 }));
    vi.setSystemTime(6_000);
    // Same counters as the previous tick: exactly what a 304 delivers.
    history.append(makeStatus({ total_requests: 100 }));
    expect(history.samples[1].reqPerSec).toBe(0);
  });

  it('clamps a counter reset (negative delta) to zero instead of spiking', () => {
    vi.useFakeTimers();
    vi.setSystemTime(1_000);
    history.append(makeStatus({ total_requests: 200 }));
    vi.setSystemTime(6_000);
    // start_time UNCHANGED but counters went backwards: a malformed backend or
    // a counter wrap. Must not render as a negative rate.
    history.append(makeStatus({ total_requests: 50 }));
    expect(history.samples[1].reqPerSec).toBe(0);
  });
});

describe('history.append restart detection', () => {
  it('drops the buffer and re-seeds when start_time changes', () => {
    vi.useFakeTimers();
    vi.setSystemTime(1_000);
    history.append(makeStatus({ start_time: '2026-01-01T00:00:00Z', total_requests: 100 }));
    vi.setSystemTime(6_000);
    history.append(makeStatus({ start_time: '2026-01-01T00:00:00Z', total_requests: 110 }));
    expect(history.length).toBe(2);

    // Process restarted: new era, counters reset, history must drop.
    vi.setSystemTime(11_000);
    history.append(makeStatus({ start_time: '2026-01-02T00:00:00Z', total_requests: 5 }));
    expect(history.length).toBe(1);
    expect(history.samples[0].totalRequests).toBe(5);
    expect(history.samples[0].reqPerSec).toBe(0); // fresh zero-delta baseline
  });

  it('re-seeds the baseline when the gap since the last sample is large', () => {
    vi.useFakeTimers();
    vi.setSystemTime(1_000);
    history.append(makeStatus({ total_requests: 100 }));
    // Panel hidden for 60 s: the next delta would be a huge per-second spike
    // over a period the user wasn't even watching.
    vi.setSystemTime(61_000);
    history.append(makeStatus({ total_requests: 600 }));
    expect(history.samples[1].reqPerSec).toBe(0); // stale baseline re-seeded
  });
});

describe('history ring buffer', () => {
  it('caps at MAX_SAMPLES and evicts the oldest', () => {
    for (let i = 0; i <= MAX_SAMPLES; i++) {
      history.append(makeStatus({ total_requests: i }));
    }
    expect(history.length).toBe(MAX_SAMPLES);
    // The i=0 sample was evicted; i=1 is now the oldest.
    expect(history.samples[0].totalRequests).toBe(1);
    expect(history.samples[MAX_SAMPLES - 1].totalRequests).toBe(MAX_SAMPLES);
  });
});
