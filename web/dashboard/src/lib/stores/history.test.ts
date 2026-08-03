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

describe('history.appendError (failed-poll markers)', () => {
  it('pushes an error sample with no counter data', () => {
    vi.useFakeTimers();
    vi.setSystemTime(1_000);
    history.appendError();
    expect(history.length).toBe(1);
    expect(history.samples[0].error).toBe(true);
    expect(history.samples[0].reqPerSec).toBe(0);
    expect(history.samples[0].activeConnections).toBe(0);
  });

  it('does NOT update prev: the next good sample deltas against the last good baseline', () => {
    vi.useFakeTimers();
    vi.setSystemTime(1_000);
    history.append(makeStatus({ total_requests: 100 }));
    vi.setSystemTime(6_000);
    history.appendError(); // outage tick at 6 s
    vi.setSystemTime(11_000);
    history.append(makeStatus({ total_requests: 130 }));
    // prev was NOT updated by the error tick, so the delta is 30 requests
    // over 10 s (1 s -> 11 s) = 3 req/s, not a counter-reset zero.
    expect(history.samples[2].reqPerSec).toBe(3);
  });

  it('keeps the stale-gap guard inactive across a visible outage', () => {
    vi.useFakeTimers();
    vi.setSystemTime(1_000);
    history.append(makeStatus({ total_requests: 100 }));
    // Simulate 5 consecutive failed ticks at 5 s cadence (25 s total, under
    // the 30 s stale-gap threshold but close). Each error updates lastSampleT.
    for (let s = 6_000; s <= 26_000; s += 5_000) {
      vi.setSystemTime(s);
      history.appendError();
    }
    // Recovery tick at 31 s. Without the lastSampleT fix, prev.t (1 s) is
    // >30 s ago and the stale-gap guard would zero the baseline, losing the
    // real rate across the outage. The error ticks kept the timeline alive.
    vi.setSystemTime(31_000);
    history.append(makeStatus({ total_requests: 250 }));
    const last = history.samples[history.samples.length - 1];
    // 150 requests over 30 s = 5 req/s. If the guard had fired, this would
    // be 0.
    expect(last.reqPerSec).toBe(5);
  });

  it('does not trigger restart detection (start_time unchanged by errors)', () => {
    vi.useFakeTimers();
    vi.setSystemTime(1_000);
    history.append(makeStatus({ start_time: '2026-01-01T00:00:00Z', total_requests: 100 }));
    vi.setSystemTime(6_000);
    history.appendError();
    vi.setSystemTime(11_000);
    // Same start_time: must NOT reset the buffer.
    history.append(makeStatus({ start_time: '2026-01-01T00:00:00Z', total_requests: 110 }));
    expect(history.length).toBe(3); // not reset
  });

  it('error samples are capped by the ring buffer like any other', () => {
    for (let i = 0; i <= MAX_SAMPLES; i++) {
      history.appendError();
    }
    expect(history.length).toBe(MAX_SAMPLES);
    expect(history.samples.every((s) => s.error)).toBe(true);
  });
});
