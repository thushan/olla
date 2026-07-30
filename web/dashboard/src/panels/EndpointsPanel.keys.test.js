import { flushSync, mount, unmount } from 'svelte';
import { describe, it, expect, afterEach, vi } from 'vitest';
import { endpoints } from '../lib/stores/endpoints.svelte.js';
import EndpointsPanel from './EndpointsPanel.svelte';

// Regression coverage: two endpoints whose names differ only in punctuation
// ("node.a" / "node-a") both slugged to the same CSS-safe id ("node-a"). The
// panel used that lossy slug as the keyed-each identity, so Svelte's
// each_key_duplicate error fired inside a reactive effect and - with no error
// boundary anywhere in the app - blanked the ENTIRE table body, not just the
// colliding rows. Real-world trigger: EndpointConfig.Name is a free-form
// string with no charset restriction, so this is one YAML file away from
// happening in production.

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

async function refreshWithEndpoints(endpointList) {
  global.fetch = vi.fn(async () =>
    jsonResponse({
      endpoints: endpointList,
      total_count: endpointList.length,
      healthy_count: endpointList.length,
      routable_count: endpointList.length,
    })
  );
  endpoints.refresh();
  const lastName = endpointList[endpointList.length - 1].name;
  await vi.waitFor(() =>
    expect(endpoints.data?.endpoints?.[endpointList.length - 1]?.name).toBe(lastName)
  );
  flushSync();
}

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

describe('EndpointsPanel row keys', () => {
  it('renders both rows when two endpoint names collide once slugged to a CSS-safe id', async () => {
    component = mount(EndpointsPanel, { target: document.body });
    flushSync();

    await refreshWithEndpoints([endpointNamed('node.a'), endpointNamed('node-a')]);

    const nameCells = [...document.querySelectorAll('.name-text')].map((el) => el.textContent.trim());
    expect(nameCells).toEqual(expect.arrayContaining(['node.a', 'node-a']));
    expect(nameCells.length).toBe(2);
  });
});
