import { flushSync, mount, unmount } from 'svelte';
import { describe, it, expect, afterEach, vi } from 'vitest';
import { overview } from '../lib/stores/overview.svelte.js';
import { endpoints } from '../lib/stores/endpoints.svelte.js';
import OverviewPanel from './OverviewPanel.svelte';

// Regression coverage for finding 8: the glance table rendered fmtMs(0) as a
// confident "0ms" for an endpoint with no request history, indistinguishable
// from genuinely fast traffic. Must gate on request_count > 0, mirroring the
// PctBar's hasData pattern two lines above. Covers both wire shapes so this
// keeps working whichever lands first: avg_latency_ms present-but-zero (today)
// and avg_latency_ms absent/null (once the backend field becomes *int64 with
// omitempty).

let component;
afterEach(() => {
  if (component) unmount(component);
  document.body.innerHTML = '';
});

function jsonResponse(body) {
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
    version: 'v0.0.29',
    commit: 'abc123',
    start_time: new Date().toISOString(),
  },
  proxy: { engine: 'olla', balancer: 'priority' },
};

async function refreshBoth(endpointList) {
  global.fetch = vi.fn(async (url) => {
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
    expect(cell.textContent.trim()).toBe('—');
    expect(cell.textContent.trim()).not.toBe('0ms');
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
    expect(cell.textContent.trim()).toBe('—');
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
    expect(cell.textContent.trim()).toBe('55ms');
  });
});
