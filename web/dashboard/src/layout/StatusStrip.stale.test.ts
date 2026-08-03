import { flushSync, mount, unmount } from 'svelte';
import { describe, it, expect, afterEach, vi } from 'vitest';
import type { SystemSummary, EndpointResponse, PollStatus } from '../lib/types';

// C6 regression: StatusStrip read overview.data without consulting
// overview.status, so during an outage it displayed confident numbers while
// the panels below reported unreachable. The fix surfaces a stale/error
// indicator on the strip when overview.status is 'stale' or 'error', reusing
// the data-state pattern the panels already use.
//
// The overview store's `status` is closure-private with no setter, so this
// test substitutes a controllable fake store to exercise the component's
// reaction to each status value in isolation.

interface Holder {
  status: PollStatus;
  sys: SystemSummary;
  endpoints: EndpointResponse[];
}

const holder = vi.hoisted<Holder>(() => ({
  status: 'ok',
  sys: {
    status: 'healthy',
    endpoints_up: '2/2',
    success_rate: '99.0%',
    avg_latency: '20ms',
    total_traffic: '0',
    uptime: '0s',
    version: 'v0.0.29',
    commit: 'abc',
    start_time: new Date().toISOString(),
    active_connections: 0,
    security_violations: 0,
    total_requests: 0,
    total_failures: 0,
    has_traffic: false,
  },
  endpoints: [],
}));

vi.mock('../lib/stores/overview.svelte', () => ({
  overview: {
    get data() {
      return { system: holder.sys, endpoints: holder.endpoints };
    },
    get status() {
      return holder.status;
    },
  },
}));

// The strip no longer reads the endpoints store (it was pausable, which is
// what made the degraded reason go stale). Keep the mock minimal so a stray
// reference inside the component surfaces as an explicit error rather than a
// silent undefined.
vi.mock('../lib/stores/endpoints.svelte', () => ({
  endpoints: {
    get data() {
      throw new Error('StatusStrip must not read the endpoints store');
    },
  },
}));

// Imported after the mocks are registered.
const StatusStripModule = await import('./StatusStrip.svelte');
const StatusStrip = StatusStripModule.default;

let component: ReturnType<typeof mount> | undefined;
afterEach(() => {
  if (component) unmount(component);
  document.body.innerHTML = '';
  holder.status = 'ok';
  holder.endpoints = [];
});

describe('StatusStrip staleness indicator', () => {
  it('shows a stale indicator when overview.status is "stale" with data present', () => {
    holder.status = 'stale';
    component = mount(StatusStrip, { target: document.body });
    flushSync();

    const strip = document.querySelector('.status-strip');
    expect(strip).toBeTruthy();
    // data-state mirrors the panels' pattern so CSS/AT can distinguish a
    // confident strip from a stale one.
    expect(strip!.getAttribute('data-state')).toBe('stale');
    // A visible stale marker is surfaced, not just an attribute.
    expect(strip!.querySelector('[data-stale]')!.textContent!.trim().length).toBeGreaterThan(0);
  });

  it('shows an error indicator when overview.status is "error"', () => {
    holder.status = 'error';
    component = mount(StatusStrip, { target: document.body });
    flushSync();

    const strip = document.querySelector('.status-strip')!;
    expect(strip.getAttribute('data-state')).toBe('error');
    expect(strip.querySelector('[data-stale]')).toBeTruthy();
  });

  it('shows no staleness marker when overview.status is "ok"', () => {
    holder.status = 'ok';
    component = mount(StatusStrip, { target: document.body });
    flushSync();

    const strip = document.querySelector('.status-strip')!;
    expect(strip.getAttribute('data-state')).toBeNull();
    expect(strip.querySelector('[data-stale]')).toBeNull();
  });
});

describe('StatusStrip degraded reason', () => {
  // The strip used to derive its offline count from the pausable endpoints
  // store, which froze mid-outage once EndpointsPanel unmounted. It now reads
  // overview.data.endpoints (always fresh because the overview store runs for
  // the lifetime of the dashboard). When that field is absent the strip must
  // stay silent rather than fabricate a count.

  it('derives the offline count from overview.data.endpoints (fresh payload)', () => {
    holder.status = 'ok';
    holder.sys.status = 'degraded';
    holder.endpoints = [
      { name: 'a', status: 'offline', url: 'u1', id: 'e1', success_rate: '0%', avg_latency: '0ms', traffic: '0', last_check: '', next_check: '', issues: '', priority: 0, connections: 0, requests: 0, min_latency_ms: 0, max_latency_ms: 0, models: { last_updated: '', count: 0 } },
      { name: 'b', status: 'healthy', url: 'u2', id: 'e2', success_rate: '100%', avg_latency: '0ms', traffic: '0', last_check: '', next_check: '', issues: '', priority: 0, connections: 0, requests: 0, min_latency_ms: 0, max_latency_ms: 0, models: { last_updated: '', count: 0 } },
      { name: 'c', status: 'critical', url: 'u3', id: 'e3', success_rate: '0%', avg_latency: '0ms', traffic: '0', last_check: '', next_check: '', issues: '', priority: 0, connections: 0, requests: 0, min_latency_ms: 0, max_latency_ms: 0, models: { last_updated: '', count: 0 } },
    ];
    component = mount(StatusStrip, { target: document.body });
    flushSync();

    // Two of three endpoints are offline/critical; the strip surfaces that
    // as the degraded reason, sourced from the always-fresh overview payload.
    const reason = document.querySelector('.status-strip .status-reason');
    expect(reason?.textContent?.trim()).toBe('2 offline');
  });

  it('suppresses the reason when the overview payload has no endpoints array', () => {
    holder.status = 'ok';
    holder.sys.status = 'degraded';
    holder.endpoints = [];
    component = mount(StatusStrip, { target: document.body });
    flushSync();

    // No endpoint data to count from - the strip stays quiet rather than
    // show a fabricated or stale count.
    const reason = document.querySelector('.status-strip .status-reason');
    expect(reason).toBeNull();
  });

  it('stays quiet when degraded but every listed endpoint is healthy', () => {
    holder.status = 'ok';
    holder.sys.status = 'degraded';
    holder.endpoints = [
      { name: 'a', status: 'healthy', url: 'u1', id: 'e1', success_rate: '100%', avg_latency: '0ms', traffic: '0', last_check: '', next_check: '', issues: '', priority: 0, connections: 0, requests: 0, min_latency_ms: 0, max_latency_ms: 0, models: { last_updated: '', count: 0 } },
    ];
    component = mount(StatusStrip, { target: document.body });
    flushSync();

    const reason = document.querySelector('.status-strip .status-reason');
    expect(reason).toBeNull();
  });
});
