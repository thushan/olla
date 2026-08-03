// Ring-buffer of overview snapshots for the SparkStrip. One sample per poll
// tick (success AND failure) via the overview store's onTick callback. Failed
// ticks push an error sample (error: true, no counter data); the chart draws
// a red marker at each error sample's x position and breaks the area/line
// paths around it so the outage is visible rather than bridged over.
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
  // true on a failed-poll marker. Error samples carry no counter data and
  // must not feed the delta baseline or restart detection.
  error?: boolean;
}

// ~15 min of samples at the 5 s overview cadence.
export const MAX_SAMPLES = 180;

// If no sample at all (success or error) arrives within this window, the
// panel was hidden or the tab was backgrounded beyond the scheduler's backoff
// cadence, so the delta baseline is stale. Error ticks update lastSampleT so
// a visible outage (where error ticks keep firing) does NOT trip this guard:
// the next successful sample legitimately deltas across the outage window.
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
// Previous SUCCESSFUL sample drives delta computation for the next. Plain
// module var, not reactive: only `samples` is read by the SVG.
let prev: Sample | null = null;
// Tracked across ticks to detect process restarts (start_time change).
let lastStartTime: string | null = null;
// t of the most recent sample of any kind (success or error). Drives the
// stale-gap guard: if this goes silent for >30s the panel was hidden.
let lastSampleT = 0;

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
   *  body is a no-op so callers can wire this directly to the tick callback. */
  append(data: StatusResponse | null): void {
    if (data === null) return;
    const sys = data.system;
    const start = sys?.start_time ?? null;

    if (detectRestart(lastStartTime, start)) {
      // Counters reset at the restart boundary, so re-seed the delta
      // baseline. The history itself is kept: past samples and outage
      // markers stay visible across a restart, so recovery doesn't erase
      // the outage the operator just lived through.
      prev = null;
    }

    const now = Date.now();
    // Stale-gap guard: only fires when NO sample (success or error) arrived
    // for STALE_GAP_MS, meaning the panel was hidden. Error ticks during an
    // outage keep lastSampleT current so this guard does not fire across a
    // visible outage and the next good sample produces a real rate.
    if (prev !== null && now - lastSampleT > STALE_GAP_MS) {
      prev = null;
    }

    const sample = buildSample(data, now);
    samples.push(sample);
    if (samples.length > MAX_SAMPLES) samples.shift();
    prev = sample;
    lastStartTime = start;
    lastSampleT = now;
  },
  /** Append a failed-poll marker. Error samples carry no counter data and
   *  deliberately do NOT update prev or lastStartTime: the next successful
   *  sample deltas against the last good prev, yielding a real rate across
   *  the outage window rather than a counter-reset spike. */
  appendError(): void {
    const now = Date.now();
    const sample: Sample = {
      t: now,
      totalRequests: 0,
      totalBytes: 0,
      activeConnections: 0,
      reqPerSec: 0,
      bytesPerSec: 0,
      error: true,
    };
    samples.push(sample);
    if (samples.length > MAX_SAMPLES) samples.shift();
    // Update lastSampleT so the stale-gap guard sees a live timeline even
    // during an outage. Do NOT touch prev or lastStartTime.
    lastSampleT = now;
  },
  /** Drop everything: samples, delta baseline, start_time tracking. */
  reset(): void {
    samples = [];
    prev = null;
    lastStartTime = null;
    lastSampleT = 0;
  },
  /** Drop only the rendered samples, keeping the delta baseline. Useful for
   *  tests that want a clean array without resetting the rate tracking. */
  clear(): void {
    samples = [];
  },
};
