// Herd table logic that is independent of Svelte: which columns exist, which
// are visible, what the picker persists, and how rows are filtered. Kept out
// of the components so it can be unit tested without a DOM.

export type HerdColumnKey =
  | 'active_connections'
  | 'request_count'
  | 'type'
  | 'model_count'
  | 'priority'
  | 'probe_ms'
  | 'traffic_bytes'
  | 'failures';

export interface OptionalColumn {
  key: HerdColumnKey;
  label: string;
  // Extra detail for the picker and the header tooltip.
  hint: string;
  defaultOn: boolean;
}

// Endpoint, Status, Resp. rate and Latency are core: always rendered, never in
// the picker. Everything below is optional and is the only thing persisted.
export const OPTIONAL_COLUMNS: OptionalColumn[] = [
  { key: 'active_connections', label: 'In flight', hint: 'Requests being served right now', defaultOn: true },
  { key: 'request_count', label: 'Requests', hint: 'Requests proxied since Olla started', defaultOn: true },
  { key: 'type', label: 'Type', hint: 'Backend kind (ollama, vllm, ...)', defaultOn: true },
  { key: 'model_count', label: 'Models', hint: 'Models discovered on this endpoint', defaultOn: true },
  { key: 'priority', label: 'Priority', hint: 'Balancer priority; higher is preferred', defaultOn: false },
  {
    key: 'probe_ms',
    label: 'Probe latency',
    hint: 'Latency of the last health-check probe, not of proxied requests',
    defaultOn: false,
  },
  { key: 'traffic_bytes', label: 'Traffic', hint: 'Bytes served by this endpoint since start', defaultOn: false },
  {
    key: 'failures',
    label: 'Failures',
    hint: 'Estimated from requests and response rate, so it is approximate',
    defaultOn: false,
  },
];

export const COLUMNS_STORAGE_KEY = 'olla-herd-columns';

export function defaultColumns(): HerdColumnKey[] {
  return OPTIONAL_COLUMNS.filter((c) => c.defaultOn).map((c) => c.key);
}

// Keeps only keys that still exist, so a key retired in a later release (or a
// hand-edited value) cannot break the table. Anything that isn't a string
// array falls back to the defaults; an empty array is a valid choice.
export function sanitiseColumns(raw: unknown): HerdColumnKey[] {
  if (!Array.isArray(raw)) return defaultColumns();
  const known = new Set<string>(OPTIONAL_COLUMNS.map((c) => c.key));
  return OPTIONAL_COLUMNS.map((c) => c.key).filter((k) => raw.includes(k) && known.has(k));
}

export function loadColumns(): HerdColumnKey[] {
  // Storage throws in private windows and when site data is blocked.
  try {
    if (typeof localStorage === 'undefined') return defaultColumns();
    const v = localStorage.getItem(COLUMNS_STORAGE_KEY);
    if (v === null) return defaultColumns();
    return sanitiseColumns(JSON.parse(v));
  } catch {
    return defaultColumns();
  }
}

export function saveColumns(keys: HerdColumnKey[]): void {
  try {
    if (typeof localStorage === 'undefined') return;
    localStorage.setItem(COLUMNS_STORAGE_KEY, JSON.stringify(keys));
  } catch {
    // Persistence is a convenience; the picker still works for this session.
  }
}

export type StatusFilter = 'all' | 'unhealthy';

// Anything not plainly healthy counts as needing attention, including
// 'unknown', which on a fresh boot means "not yet probed".
export function filterEndpoints<T extends { name: string; type?: string; status: string }>(
  rows: T[],
  query: string,
  status: StatusFilter
): T[] {
  const q = query.trim().toLowerCase();
  return rows.filter((r) => {
    if (status === 'unhealthy' && r.status === 'healthy') return false;
    if (!q) return true;
    return r.name.toLowerCase().includes(q) || (r.type ?? '').toLowerCase().includes(q);
  });
}

export function isFiltering(query: string, status: StatusFilter): boolean {
  return query.trim() !== '' || status !== 'all';
}

// Failures = requests * (1 - rate). The API gives no failure count per
// endpoint, so this is an estimate from a one-decimal percentage; null when
// there is no traffic to estimate from.
export function estimateFailures(requestCount: number, successRatePct: number): number | null {
  if (!(requestCount > 0)) return null;
  const pct = Math.min(100, Math.max(0, successRatePct));
  return Math.round(requestCount * (1 - pct / 100));
}

// response_time arrives formatted by pkg/format.Latency ("12ms", "1.5s").
// Returns null when absent or unparseable so sorting and display can tell
// "no probe yet" apart from a real 0.
export function parseProbeMs(s: string | undefined | null): number | null {
  if (!s) return null;
  const m = String(s).trim().match(/^([\d.]+)\s*(ms|s)$/);
  if (!m) return null;
  const n = parseFloat(m[1]);
  return m[2] === 's' ? n * 1000 : n;
}

// Splits the issues string ("unavailable", or a comma list on older builds).
export function issueList(issues?: string | null): string[] {
  if (!issues) return [];
  return String(issues)
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean);
}
