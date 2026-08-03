import { flushSync, mount, unmount } from 'svelte';
import { describe, it, expect, afterEach, vi } from 'vitest';
import { endpoints } from '../lib/stores/endpoints.svelte.ts';
import { models } from '../lib/stores/models.svelte.ts';
import { overview } from '../lib/stores/overview.svelte.ts';
import EndpointsPanel from './EndpointsPanel.svelte';
import ModelsPanel from './ModelsPanel.svelte';
import OverviewPanel from './OverviewPanel.svelte';

// Regression coverage for the row-key collision bug class fixed piecemeal in
// eb14006 (Endpoints/Models) and again here (Overview's glance table). Any
// SortableTable consumer that keys its each-block on a lossy CSS-safe slug,
// rather than the row's exact unique name, blanks its ENTIRE table body the
// moment two rows collide once slugged - Svelte's each_key_duplicate, with no
// error boundary anywhere in the app to contain it. "node.a" and "node-a"
// both slug to "node-a"; that pair is the fixture used throughout.
//
// Parameterised across every SortableTable consumer (Endpoints, Models
// grouped, Models flat, Overview glance) rather than one test per panel, so a
// future table added without unique rowId coverage fails here automatically
// instead of shipping the same bug a fourth time.

const COLLIDING_NAMES = ['node.a', 'node-a'];

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
    avg_latency: '20ms',
    total_traffic: '1.00 GB',
    active_connections: 0,
    total_requests: 10,
    total_failures: 0,
    security_violations: 0,
    version: 'v0.0.29',
    commit: 'abc123',
    start_time: new Date().toISOString(),
  },
  proxy: { engine: 'olla', balancer: 'priority' },
};

function endpointNamed(name) {
  return {
    name,
    type: 'ollama',
    status: 'healthy',
    priority: 100,
    success_rate: '99.0%',
    request_count: 10,
    model_count: 1,
    avg_latency_ms: 20,
    min_latency_ms: 5,
    max_latency_ms: 40,
  };
}

function modelNamed(name) {
  return {
    name,
    params: '8B',
    quant: 'q4_k_m',
    size: '4.5 GB',
    endpoints: ['ollama-1'],
    last_seen_at: new Date().toISOString(),
  };
}

const cases = [
  {
    label: 'EndpointsPanel',
    Component: EndpointsPanel,
    async seed() {
      global.fetch = vi.fn(async () =>
        jsonResponse({
          endpoints: COLLIDING_NAMES.map(endpointNamed),
          total_count: 2,
          healthy_count: 2,
          routable_count: 2,
        })
      );
      endpoints.refresh();
      await vi.waitFor(() => expect(endpoints.data?.endpoints?.[1]?.name).toBe(COLLIDING_NAMES[1]));
      flushSync();
    },
    renderedNames() {
      return [...document.querySelectorAll('.name-text')].map((el) => el.textContent.trim());
    },
  },
  {
    label: 'ModelsPanel (grouped)',
    Component: ModelsPanel,
    async seed() {
      global.fetch = vi.fn(async () =>
        jsonResponse({
          model_groups: [
            {
              family: 'qwen',
              model_count: 2,
              endpoints: ['ollama-1'],
              models: COLLIDING_NAMES.map(modelNamed),
            },
          ],
        })
      );
      models.refresh();
      await vi.waitFor(() => expect(models.data?.model_groups?.[0]?.models?.length).toBe(2));
      flushSync();
    },
    renderedNames() {
      return [...document.querySelectorAll('tbody td.col-sticky strong')].map((el) => el.textContent.trim());
    },
  },
  {
    label: 'ModelsPanel (flat recent_models)',
    Component: ModelsPanel,
    async seed() {
      global.fetch = vi.fn(async () => jsonResponse({ recent_models: COLLIDING_NAMES.map(modelNamed) }));
      models.refresh();
      await vi.waitFor(() => expect(models.data?.recent_models?.length).toBe(2));
      flushSync();
    },
    renderedNames() {
      return [...document.querySelectorAll('tbody td.col-sticky strong')].map((el) => el.textContent.trim());
    },
  },
  {
    label: 'OverviewPanel (glance table)',
    Component: OverviewPanel,
    async seed() {
      global.fetch = vi.fn(async (url) => {
        if (String(url).includes('/internal/status/endpoints')) {
          return jsonResponse({
            endpoints: COLLIDING_NAMES.map(endpointNamed),
            total_count: 2,
            healthy_count: 2,
            routable_count: 2,
          });
        }
        return jsonResponse(sysBody);
      });
      overview.refresh();
      endpoints.refresh();
      await vi.waitFor(() => {
        expect(overview.data?.system?.status).toBe('healthy');
        expect(endpoints.data?.endpoints?.[1]?.name).toBe(COLLIDING_NAMES[1]);
      });
      flushSync();
    },
    renderedNames() {
      return [...document.querySelectorAll('.glance-link .txt')].map((el) => el.textContent.trim());
    },
  },
];

let component;
afterEach(() => {
  if (component) unmount(component);
  document.body.innerHTML = '';
});

describe.each(cases)('$label row keys survive a slug collision', ({ Component, seed, renderedNames }) => {
  it('renders both colliding rows without throwing each_key_duplicate', async () => {
    component = mount(Component, { target: document.body });
    flushSync();

    await seed();

    const names = renderedNames();
    expect(names).toEqual(expect.arrayContaining(COLLIDING_NAMES));
    expect(names.length).toBe(2);
  });
});
