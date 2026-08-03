import { flushSync, mount, unmount } from 'svelte';
import { describe, it, expect, afterEach, vi } from 'vitest';
import { overview } from '../lib/stores/overview.svelte';
import { endpoints } from '../lib/stores/endpoints.svelte';
import OverviewPanel from './OverviewPanel.svelte';
import type {
  EndpointResponse,
  EndpointSummary,
  SecuritySummary,
  SecurityViolation,
  StatusResponse,
} from '../lib/types';

// Regression coverage for finding 8: the glance table rendered fmtMs(0) as a
// confident "0ms" for an endpoint with no request history, indistinguishable
// from genuinely fast traffic. Must gate on request_count > 0, mirroring the
// PctBar's hasData pattern two lines above. Covers both wire shapes so this
// keeps working whichever lands first: avg_latency_ms present-but-zero (today)
// and avg_latency_ms absent/null (once the backend field becomes *int64 with
// omitempty).

let component: ReturnType<typeof mount> | undefined;
afterEach(() => {
  if (component) unmount(component);
  document.body.innerHTML = '';
});

function jsonResponse(body: unknown) {
  return {
    status: 200,
    ok: true,
    headers: { get: () => null },
    json: async () => body,
  };
}

const sysBody = {
  system: {
    status: 'healthy',
    endpoints_up: '2/2',
    success_rate: '99.0%',
    avg_latency: '42ms',
    total_traffic: '1.00 GB',
    active_connections: 0,
    total_requests: 40,
    total_failures: 0,
    security_violations: 7,
    version: 'v0.0.29',
    commit: 'abc123',
    start_time: new Date().toISOString(),
  },
  proxy: { engine: 'olla', balancer: 'priority' },
};

async function refreshBoth(endpointList: Record<string, unknown>[]) {
  global.fetch = vi.fn(async (url: RequestInfo | URL) => {
    if (String(url).includes('/internal/status/endpoints')) {
      return jsonResponse({
        endpoints: endpointList,
        total_count: endpointList.length,
        healthy_count: endpointList.length,
        routable_count: endpointList.length,
      });
    }
    return jsonResponse(sysBody);
  });
  overview.refresh();
  endpoints.refresh();
  const lastName = endpointList[endpointList.length - 1].name;
  await vi.waitFor(() => {
    expect(overview.data?.system?.status).toBe('healthy');
    expect(endpoints.data?.endpoints?.[endpointList.length - 1]?.name).toBe(lastName);
  });
  flushSync();
}

// Typed variant for the new-tile coverage: full StatusResponse and
// EndpointSummary fixtures, no `any`, so this compiles under strict TS and
// stays in lockstep with the contract if it shifts.
function buildStatus(over: Partial<StatusResponse> = {}): StatusResponse {
  const violations: SecurityViolation = { rate_limits: 3, size_limits: 4, ...over.security?.violations };
  const security: SecuritySummary = {
    status: 'normal',
    blocked_ips: 0,
    violations,
    ...over.security,
  };
  return {
    timestamp: new Date().toISOString(),
    proxy: { engine: 'olla', profile: 'balanced', balancer: 'priority', ...over.proxy },
    endpoints: [],
    security,
    system: {
      start_time: new Date().toISOString(),
      status: 'healthy',
      endpoints_up: '2/2',
      success_rate: '99.0%',
      avg_latency: '42ms',
      total_traffic: '1.00 GB',
      uptime: '1h',
      version: 'v0.0.29',
      commit: 'abc123',
      active_connections: 0,
      total_requests: 40,
      total_failures: 0,
      security_violations: 7,
      ...over.system,
    },
  };
}

function buildEndpoint(over: Partial<EndpointSummary>): EndpointSummary {
  return {
    name: 'ep',
    type: 'ollama',
    status: 'healthy',
    success_rate: '99.0%',
    health_check: '5s ago',
    last_model_sync: '5s ago',
    url: 'http://node:11434',
    id: 'ep-1',
    priority: 100,
    model_count: 1,
    request_count: 10,
    min_latency_ms: 0,
    max_latency_ms: 0,
    active_connections: 0,
    ...over,
  };
}

async function refreshTyped(status: StatusResponse, list: EndpointSummary[]) {
  global.fetch = vi.fn(async (url: RequestInfo | URL) => {
    if (String(url).includes('/internal/status/endpoints')) {
      return jsonResponse({
        endpoints: list,
        total_count: list.length,
        healthy_count: list.length,
        routable_count: list.length,
      });
    }
    return jsonResponse(status);
  });
  overview.refresh();
  endpoints.refresh();
  // Gate on the last endpoint's name, not just length: consecutive tests
  // often reuse the same count, and a length-only guard exits on the prior
  // test's stale data before this refresh lands.
  const lastName = list[list.length - 1]?.name;
  await vi.waitFor(() => {
    expect(overview.data?.system?.status).toBe(status.system.status);
    if (list.length === 0) {
      expect(endpoints.data?.endpoints?.length ?? 0).toBe(0);
    } else {
      expect(endpoints.data?.endpoints?.[list.length - 1]?.name).toBe(lastName);
    }
  });
  flushSync();
}

function tileByLabel(label: string): Element | undefined {
  return [...document.querySelectorAll('.tile')].find(
    (t) => t.querySelector('.label')?.textContent?.trim() === label
  );
}

describe('OverviewPanel glance table latency', () => {
  it('shows a no-data placeholder, not "0ms", for a zero-latency endpoint with no requests', async () => {
    component = mount(OverviewPanel, { target: document.body });
    flushSync();

    await refreshBoth([
      {
        name: 'idle-zero',
        status: 'healthy',
        success_rate: '0.0%',
        request_count: 0,
        avg_latency_ms: 0,
      },
    ]);

    const cell = document.querySelector('.glance-link .txt')?.closest('tr')?.querySelectorAll('td.num')[1];
    expect(cell!.textContent!.trim()).toBe('—');
    expect(cell!.textContent!.trim()).not.toBe('0ms');
  });

  it('shows a no-data placeholder when avg_latency_ms is absent (nullable backend field)', async () => {
    component = mount(OverviewPanel, { target: document.body });
    flushSync();

    await refreshBoth([
      {
        name: 'idle-absent',
        status: 'healthy',
        success_rate: '0.0%',
        request_count: 0,
        // no avg_latency_ms field at all
      },
    ]);

    const cell = document.querySelector('.glance-link .txt')?.closest('tr')?.querySelectorAll('td.num')[1];
    expect(cell!.textContent!.trim()).toBe('—');
  });

  it('still renders a real latency figure for an endpoint with actual traffic', async () => {
    component = mount(OverviewPanel, { target: document.body });
    flushSync();

    await refreshBoth([
      {
        name: 'active-endpoint',
        status: 'healthy',
        success_rate: '99.0%',
        request_count: 40,
        avg_latency_ms: 55,
      },
    ]);

    const cell = document.querySelector('.glance-link .txt')?.closest('tr')?.querySelectorAll('td.num')[1];
    expect(cell!.textContent!.trim()).toBe('55ms');
  });
});

describe('OverviewPanel response-rate tile is honest about what it counts (D2)', () => {
  // The underlying metric (proxy/core/base.go) counts any completed streamed
  // response as success regardless of HTTP status, so an all-500 fleet can
  // read 100%. Fixing that metric is deferred to a later change; this only
  // asserts the dashboard's own job - the tile must not present an
  // unqualified percentage. A title attribute is not enough (mouse-only,
  // invisible to a scanning operator), so this checks the rendered text
  // content, not an attribute.
  it('does not label the tile "Success rate" and renders a visible caveat', async () => {
    component = mount(OverviewPanel, { target: document.body });
    flushSync();

    await refreshBoth([
      { name: 'ep', status: 'healthy', success_rate: '99.0%', request_count: 7, avg_latency_ms: 5 },
    ]);

    // sysBody (module-level, shared by every test in this file) fixes the
    // system-level success_rate at '99.0%' - that is the tile under test,
    // not the per-endpoint figure in the glance table.
    const tiles = [...document.querySelectorAll('.tile')];
    const rateTile = tiles.find((t) => t.querySelector('.value')?.textContent!.includes(sysBody.system.success_rate))!;
    expect(rateTile).toBeTruthy();

    // Not an unqualified "Success rate" label.
    const label = rateTile.querySelector('.label')?.textContent;
    expect(label).not.toBe('Success rate');

    // The caveat is in the tile's own rendered text - no title attribute
    // needed to find it, and no {@html} sink.
    const tileText = rateTile.textContent!;
    expect(tileText).toMatch(/HTTP status/i);
    expect(tileText).toMatch(/regardless/i);
  });
});

describe('OverviewPanel security-violations tile', () => {
  it('renders the security-violations count from sys.security_violations', async () => {
    component = mount(OverviewPanel, { target: document.body });
    flushSync();

    await refreshBoth([
      { name: 'ep', status: 'healthy', success_rate: '100.0%', request_count: 1, avg_latency_ms: 5 },
    ]);

    const tiles = [...document.querySelectorAll('.stat-tile, .tile')];
    // Match by label text across whatever wrapper class the StatTile component
    // renders, so this does not break on a class rename.
    const sec = tiles.find((t) => t.textContent!.includes('Security violations'))!;
    expect(sec).toBeTruthy();
    expect(sec.textContent!).toContain('7');
  });
});

describe('OverviewPanel discovered-models tile', () => {
  it('sums model_count across the endpoints store', async () => {
    component = mount(OverviewPanel, { target: document.body });
    flushSync();

    await refreshTyped(
      buildStatus(),
      [
        buildEndpoint({ name: 'a', model_count: 4 }),
        buildEndpoint({ name: 'b', model_count: 2 }),
        buildEndpoint({ name: 'c', model_count: 0 }),
      ]
    );

    const tile = tileByLabel('Discovered models');
    expect(tile).toBeTruthy();
    const value = tile!.querySelector('.value')?.textContent?.trim();
    expect(value).toBe('6');
    expect(tile!.textContent).toMatch(/across 3 endpoints/);
  });

  it('renders zero across zero endpoints without throwing', async () => {
    component = mount(OverviewPanel, { target: document.body });
    flushSync();

    await refreshTyped(buildStatus(), []);

    const tile = tileByLabel('Discovered models');
    expect(tile).toBeTruthy();
    expect(tile!.querySelector('.value')?.textContent?.trim()).toBe('0');
    expect(tile!.textContent).toMatch(/across 0 endpoints/);
  });
});

describe('OverviewPanel latency-range tile', () => {
  it('shows the no-data placeholder, not "0ms", when every endpoint is idle', async () => {
    component = mount(OverviewPanel, { target: document.body });
    flushSync();

    await refreshTyped(
      buildStatus(),
      [
        buildEndpoint({ name: 'a', min_latency_ms: 0, max_latency_ms: 0 }),
        buildEndpoint({ name: 'b', min_latency_ms: 0, max_latency_ms: 0 }),
      ]
    );

    const tile = tileByLabel('Latency range');
    expect(tile).toBeTruthy();
    const value = tile!.querySelector('.value')?.textContent?.trim();
    expect(value).toContain('—');
    expect(value).not.toContain('0ms');
  });

  it('renders fleet min-max from per-endpoint min/max when traffic exists', async () => {
    component = mount(OverviewPanel, { target: document.body });
    flushSync();

    await refreshTyped(
      buildStatus(),
      [
        buildEndpoint({ name: 'fast-active', min_latency_ms: 12, max_latency_ms: 80 }),
        buildEndpoint({ name: 'slow-active', min_latency_ms: 40, max_latency_ms: 1500 }),
      ]
    );

    const tile = tileByLabel('Latency range');
    expect(tile).toBeTruthy();
    const value = tile!.querySelector('.value')?.textContent?.trim();
    expect(value).toBe('12ms–1.5s');
  });

  it('ignores idle endpoints when reducing the fleet min (mixed idle/busy)', async () => {
    // An idle endpoint arrives as min=max=0; if it entered the min reduction
    // the tile would read "0ms-80ms", presenting an empty measurement as a
    // real fast request. The fleet min must come from endpoints with traffic.
    component = mount(OverviewPanel, { target: document.body });
    flushSync();

    await refreshTyped(
      buildStatus(),
      [
        buildEndpoint({ name: 'idle', min_latency_ms: 0, max_latency_ms: 0 }),
        buildEndpoint({ name: 'busy', min_latency_ms: 12, max_latency_ms: 80 }),
      ]
    );

    const tile = tileByLabel('Latency range');
    expect(tile).toBeTruthy();
    const value = tile!.querySelector('.value')?.textContent?.trim();
    // Exact-match: any "0ms" leaking from the idle endpoint would surface as
    // a "0ms-80ms" floor. "80ms" itself contains the substring "0ms", so the
    // exact equality is the load-bearing assertion here, not a substring deny.
    expect(value).toBe('12ms–80ms');
  });
});

describe('OverviewPanel backend-types tile', () => {
  it('counts by type, sorted by count desc then name', async () => {
    component = mount(OverviewPanel, { target: document.body });
    flushSync();

    await refreshTyped(
      buildStatus(),
      [
        buildEndpoint({ name: 'a', type: 'ollama' }),
        buildEndpoint({ name: 'b', type: 'ollama' }),
        buildEndpoint({ name: 'c', type: 'ollama' }),
        buildEndpoint({ name: 'd', type: 'lm-studio' }),
        buildEndpoint({ name: 'e', type: 'lm-studio' }),
        buildEndpoint({ name: 'f', type: 'openai' }),
      ]
    );

    const tile = tileByLabel('Backend types');
    expect(tile).toBeTruthy();
    const value = tile!.querySelector('.value')?.textContent?.trim();
    expect(value).toBe('3 ollama · 2 lm-studio · 1 openai');
  });

  it('falls back to the no-data dash on an empty fleet', async () => {
    component = mount(OverviewPanel, { target: document.body });
    flushSync();

    await refreshTyped(buildStatus(), []);

    const tile = tileByLabel('Backend types');
    expect(tile).toBeTruthy();
    expect(tile!.querySelector('.value')?.textContent?.trim()).toContain('—');
  });
});

describe('OverviewPanel security detail in sub', () => {
  it('surfaces status, blocked IPs and the rate/size split', async () => {
    component = mount(OverviewPanel, { target: document.body });
    flushSync();

    await refreshTyped(
      buildStatus({
        security: {
          status: 'elevated',
          blocked_ips: 2,
          violations: { rate_limits: 5, size_limits: 9 },
        },
      }),
      [buildEndpoint({ name: 'ep' })]
    );

    const tile = tileByLabel('Security violations');
    expect(tile).toBeTruthy();
    const sub = tile!.querySelector('.sub')?.textContent?.trim();
    expect(sub).toContain('elevated');
    expect(sub).toContain('2 blocked IPs');
    expect(sub).toMatch(/5 rate/);
    expect(sub).toMatch(/9 size/);
  });
});

describe('OverviewPanel system-status tile (profile)', () => {
  it('renders the proxy profile when present', async () => {
    component = mount(OverviewPanel, { target: document.body });
    flushSync();

    await refreshTyped(buildStatus(), [buildEndpoint({ name: 'ep' })]);

    const tile = tileByLabel('System status');
    expect(tile).toBeTruthy();
    expect(tile!.querySelector('.sub')?.textContent).toMatch(/balanced profile/);
  });
});
