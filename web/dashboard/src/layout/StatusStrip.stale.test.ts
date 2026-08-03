import { flushSync, mount, unmount } from 'svelte';
import { describe, it, expect, afterEach, vi } from 'vitest';
import type { SystemSummary, EndpointSummary, PollStatus } from '../lib/types';

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
  },
}));

vi.mock('../lib/stores/overview.svelte', () => ({
  overview: {
    get data() {
      return { system: holder.sys };
    },
    get status() {
      return holder.status;
    },
  },
}));

vi.mock('../lib/stores/endpoints.svelte', () => ({
  endpoints: {
    get data() {
      return { endpoints: [] as EndpointSummary[] };
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
