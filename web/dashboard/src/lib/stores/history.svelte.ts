// Ring-buffer of overview snapshots for the SparkStrip. One sample per poll
// tick (200 AND 304 - see poll-store onSuccess, which advances lastUpdated in
// both branches). The 304 path is the reason this store keys off lastUpdated
// rather than data identity: on a 304 the data object is the SAME reference,
// so reading it alone would not retrigger the strip's sampling effect.
//
// Singleton module-scope state so every importer (component + tests) shares
// one buffer, matching how overview/theme are wired elsewhere.

import type { StatusResponse } from '../types';

export interface Sample {
  // epoch ms
  t: number;
  totalRequests: number;
  totalBytes: number;
  activeConnections: number;
  // derived per-tick rates
  reqPerSec: number;
  bytesPerSec: number;
}

// ~15 min of samples at the 5 s overview cadence.
export const MAX_SAMPLES = 180;

// If the gap between samples exceeds this, the previous baseline is stale
// (panel was hidden, tab backgrounded) and the next delta would misrepresent
// the rate. Re-seed the baseline silently rather than emitting a spike.
const STALE_GAP_MS = 30_000;

// --- pure helpers (exported for unit testing) ---

/** Clamp negative deltas to 0: a counter reset must not render as a negative spike. */
export function clampNonNegative(n: number): number {
  return Number.isFinite(n) && n > 0 ? n : 0;
}

/** Per-second rate from a raw counter delta over dtMs. */
export function computeRate(delta: number, dtMs: number): number {
  if (!Number.isFinite(delta) || !Number.isFinite(dtMs) || dtMs <= 0) return 0;
  return clampNonNegative(delta) / (dtMs / 1000);
}

/** True when the herd's start_time changed between snapshots: the process
 *  restarted and every counter reset, so historical deltas are meaningless
 *  and the buffer must be re-seeded from the new era. */
export function detectRestart(prevStart: string | null, nextStart: string | null): boolean {
  if (!prevStart || !nextStart) return false;
  return prevStart !== nextStart;
}

// Re-parses total_traffic ("172.02 GB") back to raw bytes so it can be
// delta'd. Mirrors the local helper in OverviewPanel; kept here so the store
// is self-contained and the parsing is unit-testable in isolation.
export function parseTrafficBytes(s: string | undefined): number {
  if (!s) return 0;
  const m = String(s).match(/^([\d.]+)\s*(B|KB|MB|GB|TB|PB)$/);
  if (!m) return 0;
  const units: Record<string, number> = {
    B: 1,
    KB: 1024,
    MB: 1024 ** 2,
    GB: 1024 ** 3,
    TB: 1024 ** 4,
    PB: 1024 ** 5,
  };
  return parseFloat(m[1]) * units[m[2]];
}

// --- singleton state ---

let samples: Sample[] = $state([]);
// Previous sample drives delta computation for the next. Plain module var,
// not reactive: only `samples` is read by the SVG.
let prev: Sample | null = null;
// Tracked across ticks to detect process restarts (start_time change).
let lastStartTime: string | null = null;

function buildSample(data: StatusResponse, now: number): Sample {
  const sys = data.system;
  const totalRequests = typeof sys.total_requests === 'number' ? sys.total_requests : 0;
  const totalBytes = parseTrafficBytes(sys.total_traffic);
  const activeConnections = typeof sys.active_connections === 'number' ? sys.active_connections : 0;

  let reqPerSec = 0;
  let bytesPerSec = 0;
  if (prev !== null) {
    // Floor dt at 1 ms: two ticks landing in the same millisecond would
    // otherwise divide by zero and the rate helpers return 0 anyway, but the
    // guard keeps the contract explicit.
    const dt = Math.max(1, now - prev.t);
    reqPerSec = computeRate(totalRequests - prev.totalRequests, dt);
    bytesPerSec = computeRate(totalBytes - prev.totalBytes, dt);
  }
  return { t: now, totalRequests, totalBytes, activeConnections, reqPerSec, bytesPerSec };
}

export const history = {
  get samples(): Sample[] {
    return samples;
  },
  get lastStartTime(): string | null {
    return lastStartTime;
  },
  get length(): number {
    return samples.length;
  },
  /** Derive a sample from the latest status response and append it. A null
   *  body (e.g. the poll store handing us nothing on a fresh mount) is a
   *  no-op so callers can wire this directly to the tick hook. */
  append(data: StatusResponse | null): void {
    if (data === null) return;
    const sys = data.system;
    const start = sys?.start_time ?? null;

    if (detectRestart(lastStartTime, start)) {
      // Counters reset at the boundary: drop pre-restart history and re-seed
      // a zero-delta baseline from this snapshot before recording it.
      samples = [];
      prev = null;
    }

    const now = Date.now();
    if (prev !== null && now - prev.t > STALE_GAP_MS) {
      prev = null;
    }

    const sample = buildSample(data, now);
    samples.push(sample);
    if (samples.length > MAX_SAMPLES) samples.shift();
    prev = sample;
    lastStartTime = start;
  },
  /** Drop everything: samples, delta baseline, start_time tracking. */
  reset(): void {
    samples = [];
    prev = null;
    lastStartTime = null;
  },
  /** Drop only the rendered samples, keeping the delta baseline. Useful for
   *  tests that want a clean array without resetting the rate tracking. */
  clear(): void {
    samples = [];
  },
};
