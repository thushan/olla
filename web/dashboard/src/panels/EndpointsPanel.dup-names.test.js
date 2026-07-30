import { flushSync, mount, unmount } from 'svelte';
import { describe, it, expect, afterEach, vi } from 'vitest';
import { endpoints } from '../lib/stores/endpoints.svelte.js';
import EndpointsPanel from './EndpointsPanel.svelte';

// Regression coverage for the each_key_duplicate blanking bug. The prior fix
// moved the each-key from a lossy slug to row.name, which handles
// slug-colliding-but-distinct names ("node.a"/"node-a") but NOT two endpoints
// that share the EXACT same name (or both empty). Svelte's each_key_duplicate
// fires regardless of whether the collision comes from a slug or an identical
// value, and with no error boundary the whole table body blanks. The fix keys
// rows on row.url, which the backend guarantees unique (it keys its endpoint
// map by URL), and makes SortableTable disambiguate any remaining collision.

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

async function refreshWith(endpointList) {
  global.fetch = vi.fn(async () =>
    jsonResponse({
      endpoints: endpointList,
      total_count: endpointList.length,
      healthy_count: endpointList.length,
      routable_count: endpointList.length,
    })
  );
  endpoints.refresh();
  await vi.waitFor(() =>
    expect(endpoints.data?.endpoints?.length).toBe(endpointList.length)
  );
  flushSync();
}

describe('EndpointsPanel exact-duplicate and empty names', () => {
  it('renders every row when two endpoints share the exact same name', async () => {
    component = mount(EndpointsPanel, { target: document.body });
    flushSync();

    await refreshWith([
      {
        name: 'ollama',
        url: 'http://node-a:11434',
        type: 'ollama',
        status: 'healthy',
        priority: 100,
        success_rate: '99.0%',
        request_count: 10,
        model_count: 1,
        avg_latency_ms: 20,
        min_latency_ms: 5,
        max_latency_ms: 40,
      },
      {
        name: 'ollama',
        url: 'http://node-b:11434',
        type: 'ollama',
        status: 'healthy',
        priority: 90,
        success_rate: '98.0%',
        request_count: 8,
        model_count: 1,
        avg_latency_ms: 25,
        min_latency_ms: 5,
        max_latency_ms: 40,
      },
    ]);

    const rows = [...document.querySelectorAll('tbody tr')];
    expect(rows.length).toBe(2);
    // No each_key_duplicate blanking: both URL cells are present.
    const urls = rows.map((r) => r.querySelector('.url-cell')?.textContent.trim());
    expect(urls).toEqual(expect.arrayContaining(['http://node-a:11434', 'http://node-b:11434']));
  });

  it('renders every row when two endpoints both have empty names', async () => {
    component = mount(EndpointsPanel, { target: document.body });
    flushSync();

    await refreshWith([
      {
        name: '',
        url: 'http://node-a:11434',
        type: 'ollama',
        status: 'healthy',
        priority: 100,
        success_rate: '99.0%',
        request_count: 10,
        model_count: 1,
      },
      {
        name: '',
        url: 'http://node-b:11434',
        type: 'ollama',
        status: 'healthy',
        priority: 90,
        success_rate: '98.0%',
        request_count: 8,
        model_count: 1,
      },
    ]);

    const rows = [...document.querySelectorAll('tbody tr')];
    expect(rows.length).toBe(2);
  });
});
