// Mirrors pkg/format.Bytes so the wire JSON and the UI never disagree on
// units, precision or labels. Keep this in lockstep with the Go function.

const BYTES_UNITS = ['KB', 'MB', 'GB', 'TB', 'PB'];
const BYTES_UNIT = 1024;

/**
 * Format a byte count. Matches pkg/format.Bytes exactly:
 * - < 1024 -> "<n> B" (integer, single space, capital B)
 * - otherwise -> "<value>.2f <Unit>" with unit drawn from KB,MB,GB,TB,PB
 */
export function fmtBytes(bytes) {
  if (!Number.isFinite(bytes) || bytes < 0) bytes = 0;
  bytes = Math.floor(bytes);

  if (bytes < BYTES_UNIT) {
    return `${bytes} B`;
  }

  let div = BYTES_UNIT;
  let exp = 0;
  for (let n = Math.floor(bytes / BYTES_UNIT); n >= BYTES_UNIT; n = Math.floor(n / BYTES_UNIT)) {
    div *= BYTES_UNIT;
    exp++;
  }
  if (exp >= BYTES_UNITS.length) exp = BYTES_UNITS.length - 1;

  const value = bytes / div;
  return `${value.toFixed(2)} ${BYTES_UNITS[exp]}`;
}

// Locale-independent grouping; en-AU matches en-GB comma grouping.
const INT_LOCALE = 'en-AU';

/** Integer with thousands separators. Pass null/undefined -> "0". */
export function fmtInt(n) {
  if (n === null || n === undefined || Number.isNaN(n)) return '0';
  return Math.floor(Number(n)).toLocaleString(INT_LOCALE);
}

/**
 * Percentage with one decimal, mirroring pkg/format.Percentage for non-boundary
 * values. The Go variant special-cases 0 and 100 to skip the decimal; this JS
 * version does too so the dashboard doesn't show "0.0%" vs the API's "0%".
 */
export function fmtPct(value) {
  if (!Number.isFinite(value)) return '0%';
  if (value === 0) return '0%';
  if (value === 100) return '100%';
  return `${value.toFixed(1)}%`;
}

/**
 * Latency in milliseconds. Mirrors pkg/format.Latency:
 * - 0 -> "0ms"
 * - >= 1000 -> "<x.x>s"
 * - < 10 -> single-digit form ("5ms" not "05ms"), but the JS path just prints
 *   the integer so we keep parity with the >9ms branch.
 */
export function fmtMs(ms) {
  if (!ms || ms <= 0) return '0ms';
  if (ms >= 1000) return `${(ms / 1000).toFixed(1)}s`;
  return `${Math.floor(ms)}ms`;
}

/**
 * Sqrt-scaled percentage for visual bars where small values must remain
 * legible against a much larger max (a 28ms p95 vs a 29400ms p99 in the
 * same track). Matches the mockup's scalePct.
 */
export function scalePct(value, max) {
  if (!max || value <= 0) return 0;
  return Math.min(100, Math.sqrt(value / max) * 100);
}

/** Choose a status colour bucket given a percentage and whether we have data. */
export function pctBucket(pct, hasData) {
  if (!hasData) return 'neutral';
  if (pct >= 99) return 'green';
  if (pct >= 90) return 'amber';
  return 'red';
}

// Time formatting for the status surfaces. The backend now emits absolute
// RFC3339 instants (start_time, *_at) instead of pre-formatted relative
// strings: relative strings change every second and rot the ETag even when
// the snapshot is unchanged. We mirror pkg/format.TimeDuration / TimeAgo so
// the wire form and the UI stay in lockstep.

/**
 * Coarse duration compact form, mirroring pkg/format.TimeDuration:
 * <10s single-digit, then m/h/d. Used for both ago and until.
 */
export function fmtDuration(ms) {
  if (!Number.isFinite(ms) || ms < 0) ms = 0;
  const s = ms / 1000;
  if (s < 60) {
    const whole = Math.floor(s);
    // Match the Go path: 0..9 render as a single digit, no "0" prefix.
    return `${whole < 10 ? whole : whole}s`;
  }
  if (s < 3600) return `${Math.floor(s / 60)}m`;
  if (s < 86400) return `${Math.floor(s / 3600)}h`;
  return `${Math.floor(s / 86400)}d`;
}

/**
 * "Xs ago" for an ISO timestamp; "" when missing/zero so callers can fall
 * back to a placeholder. Mirrors pkg/format.TimeAgo's "never" sentinel by
 * returning empty, leaving the placeholder decision at the call site.
 */
export function fmtAgo(iso, nowMs = Date.now()) {
  const t = parseIso(iso);
  if (t === null) return '';
  return `${fmtDuration(nowMs - t)} ago`;
}

/**
 * "in Xs" for a future instant (next_check_at); past or now -> "now".
 * Mirrors pkg/format.TimeUntil.
 */
export function fmtUntil(iso, nowMs = Date.now()) {
  const t = parseIso(iso);
  if (t === null) return '';
  const diff = t - nowMs;
  if (diff <= 0) return 'now';
  return `in ${fmtDuration(diff)}`;
}

/**
 * Uptime compact form mirroring pkg/format.Duration2: "<1m -> Xs",
 * "<1h -> Xm", "<24h -> Xh Ym", else "Xd Yh". The herd runs for days so
 * the day/hour branch dominates in practice.
 */
export function fmtUptime(iso, nowMs = Date.now()) {
  const t = parseIso(iso);
  if (t === null) return '';
  const ms = Math.max(0, nowMs - t);
  const s = ms / 1000;
  if (s < 60) return `${Math.floor(s)}s`;
  if (s < 3600) return `${Math.floor(s / 60)}m`;
  const totalMins = Math.floor(s / 60);
  const hours = Math.floor(totalMins / 60);
  const mins = totalMins % 60;
  if (s < 86400) return `${hours}h ${mins}m`;
  const days = Math.floor(hours / 24);
  const remHours = hours % 24;
  return `${days}d ${remHours}h`;
}

// Date.parse is too lenient (no format validation). Reject anything that
// doesn't carry a timezone or isn't ISO-8601-ish so a malformed payload
// doesn't silently render "53 years ago".
function parseIso(iso) {
  if (!iso || typeof iso !== 'string') return null;
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return null;
  return t;
}
